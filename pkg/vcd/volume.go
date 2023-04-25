package vcd

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/vmware/cloud-provider-for-cloud-director/pkg/vcdsdk"
	"github.com/vmware/go-vcloud-director/v2/types/v56"
)

func GetDiskRecordsOfClusterByDescription(vcdClient *vcdsdk.Client, clusterId string) ([]*types.DiskRecordType, error) {
	filter := "description==" + url.QueryEscape(clusterId)
	params := map[string]string{"type": "disk", "filter": filter, "filterEncoded": "true"}

	page := 1
	disks := make([]*types.DiskRecordType, 0)
	for {
		params["page"] = strconv.Itoa(page)
		results, err := vcdClient.VCDClient.QueryWithNotEncodedParams(nil, params)
		if err != nil {
			return nil, fmt.Errorf("failed to list disk records by description. description[%s] page[%d] [%v]",
				clusterId, page, err)
		}
		disks = append(disks, results.Results.DiskRecord...)

		if !nextPageExists(results.Results.Link) {
			break
		}
		page++
	}

	return disks, nil
}

func GetDiskByHref(vcdClient *vcdsdk.Client, diskHref string) (*types.Disk, error) {
	disk := &types.Disk{}

	_, err := vcdClient.VCDClient.Client.ExecuteRequestWithApiVersion(diskHref, http.MethodGet,
		"", "error retrieving Disk: %#v", nil, disk, vcdClient.VCDClient.Client.APIVersion)

	return disk, err
}

func DetachFromAllVms(vcdClient *vcdsdk.Client, vAppName string, disk *types.Disk, log logr.Logger) error {
	vdcManager, err := vcdsdk.NewVDCManager(vcdClient, vcdClient.ClusterOrgName, vcdClient.ClusterOVDCName)
	if err != nil {
		return fmt.Errorf("unable to initialize vdcManager: [%v]", err)
	}

	vms, err := getAllAttachedVms(vcdClient, disk)
	if err != nil {
		return fmt.Errorf("unable to get attached VMs to disk:[%s] [%v]", disk.Name, err)
	}

	for _, vm := range vms {
		log.Info(fmt.Sprintf("Detaching [%s] from [%s]", disk.Name, vm.Name))
		err = detachDiskFromVM(vdcManager, vAppName, vm.Name, disk)
		if err != nil {
			return err
		}
	}

	// when there is a VM attached to the disk, there is no remove link on disk object initially.
	// After detaching VMs, we need to refresh disk links.
	if len(vms) > 0 {
		refreshed, err := GetDiskByHref(vcdClient, disk.HREF)
		if err != nil {
			return err
		}
		*disk = *refreshed
	}
	return nil
}

func DeleteDisk(vcdClient *vcdsdk.Client, disk *types.Disk) error {
	var deleteDiskLink *types.Link

	// Find the proper link for request
	for _, diskLink := range disk.Link {
		if diskLink.Rel == types.RelRemove {
			deleteDiskLink = diskLink
			break
		}
	}

	if deleteDiskLink == nil {
		return fmt.Errorf("could not find request URL for delete disk in disk Link")
	}

	// Return the task
	task, err := vcdClient.VCDClient.Client.ExecuteTaskRequestWithApiVersion(deleteDiskLink.HREF, http.MethodDelete,
		"", "error delete disk: %s", nil,
		vcdClient.VCDClient.Client.APIVersion)
	if err != nil {
		return fmt.Errorf("failed to execute deletion task of disk [%s]: [%v]", disk.Name, err)
	}

	err = task.WaitTaskCompletion()
	if err != nil {
		return fmt.Errorf("failed to wait for deletion task of disk [%s]: [%v]", disk.Name, err)
	}

	return nil
}

// inspired from https://github.com/vmware/cloud-director-named-disk-csi-driver/blob/6e3b7b79efdced300b4bd65dcdc98b07658fbfe7/pkg/vcdcsiclient/disks.go#L293
func getAllAttachedVms(vcdClient *vcdsdk.Client, disk *types.Disk) ([]*types.Reference, error) {
	var attachedVMLink *types.Link

	// Find the proper link for request
	for _, diskLink := range disk.Link {
		if diskLink.Type == types.MimeVMs {
			attachedVMLink = diskLink
			break
		}
	}

	if attachedVMLink == nil {
		return []*types.Reference{}, nil
	}

	// Decode request
	attachedVMs := types.Vms{}

	_, err := vcdClient.VCDClient.Client.ExecuteRequestWithApiVersion(attachedVMLink.HREF, http.MethodGet,
		attachedVMLink.Type, "error getting attached vms: %s", nil, &attachedVMs, vcdClient.VCDClient.Client.APIVersion)

	return attachedVMs.VmReference, err
}

// inspired from https://github.com/vmware/cloud-director-named-disk-csi-driver/blob/6e3b7b79efdced300b4bd65dcdc98b07658fbfe7/pkg/vcdcsiclient/disks.go#L514
func detachDiskFromVM(vdcManager *vcdsdk.VdcManager, vAppName string, vmName string, disk *types.Disk) error {

	vm, err := vdcManager.FindVMByName(vAppName, vmName)
	if err != nil {
		return fmt.Errorf("unable to get vm: [%v]", err)
	}

	params := &types.DiskAttachOrDetachParams{
		Disk: &types.Reference{HREF: disk.HREF},
	}
	task, err := vm.DetachDisk(params)
	if err != nil {
		return fmt.Errorf("unable to detack disk [%s] from vm[%s]  [%v]", disk.Name, vmName, err)
	}

	return task.WaitTaskCompletion()
}

func nextPageExists(links []*types.Link) bool {
	for _, link := range links {
		if link.Rel == "nextPage" {
			return true
		}
	}
	return false
}
