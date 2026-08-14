//go:build integration

/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package vcdfake

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// handleVersions answers the version negotiation. Only 36.0 is advertised,
// which is the version vcdsdk pins. Advertising a higher version makes govcd
// elevate some requests and changes the endpoints it uses.
func (s *Server) handleVersions(w http.ResponseWriter) {
	writeXML(w, fmt.Sprintf(`<SupportedVersions xmlns="http://www.vmware.com/vcloud/versions">
  <VersionInfo>
    <Version>%s</Version>
    <LoginUrl>%s/api/sessions</LoginUrl>
  </VersionInfo>
</SupportedVersions>`, apiVersion, s.url))
}

// handleSession authenticates. govcd reads the token from the response header
// and ignores the body.
func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	user, password, ok := r.BasicAuth()
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// VCD takes the user as `user@org`.
	name, _, _ := strings.Cut(user, "@")

	s.mu.Lock()
	s.loginUser, s.loginPassword = name, password
	s.mu.Unlock()

	if name != s.cfg.Username || password != s.cfg.Password {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	w.Header().Set("X-Vmware-Vcloud-Access-Token", accessToken)
	w.Header().Set("X-Vcloud-Authorization", accessToken)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleOrgList(w http.ResponseWriter) {
	writeXML(w, fmt.Sprintf(`<OrgList xmlns="http://www.vmware.com/vcloud/v1.5" href="%[1]s/api/org/" type="application/vnd.vmware.vcloud.orgList+xml">
  <Org href="%[1]s/api/org/%[2]s" name="%[3]s" type="application/vnd.vmware.vcloud.org+xml"/>
</OrgList>`, s.url, orgID, s.cfg.OrgName))
}

// handleOrg answers with the org. The id attribute becomes the tenant context
// header on every later swagger call.
func (s *Server) handleOrg(w http.ResponseWriter) {
	writeXML(w, fmt.Sprintf(`<Org xmlns="http://www.vmware.com/vcloud/v1.5" href="%[1]s/api/org/%[2]s" id="%[3]s" type="application/vnd.vmware.vcloud.org+xml" name="%[4]s">
  <FullName>%[4]s</FullName>
  <IsEnabled>true</IsEnabled>
</Org>`, s.url, orgID, orgURN, s.cfg.OrgName))
}

// handleQuery serves the legacy query api. One path serves several record
// types, so it dispatches on the type parameter.
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	switch rawParam(r, "type") {
	case "orgVdc", "adminOrgVdc":
		s.handleVdcQuery(w)
	case "disk":
		s.handleDiskQuery(w, r)
	default:
		s.notImplemented(w, r)
	}
}

func (s *Server) handleVdcQuery(w http.ResponseWriter) {
	writeXML(w, fmt.Sprintf(`<QueryResultRecords xmlns="http://www.vmware.com/vcloud/v1.5" type="application/vnd.vmware.vcloud.query.records+xml" name="orgVdc" page="1" pageSize="25" total="1">
  <OrgVdcRecord href="%[1]s/api/vdc/%[2]s" name="%[3]s" orgName="%[4]s" isEnabled="true" status="READY"/>
</QueryResultRecords>`, s.url, vdcID, s.cfg.VdcName, s.cfg.OrgName))
}

// handleDiskQuery returns one page of disk records. The cleaner walks the pages
// through the nextPage link, so every page but the last carries one.
func (s *Server) handleDiskQuery(w http.ResponseWriter, r *http.Request) {
	description := strings.TrimPrefix(rawParam(r, "filter"), "description==")

	page, err := strconv.Atoi(rawParam(r, "page"))
	if err != nil || page < 1 {
		page = 1
	}

	records := ""
	if page <= len(s.cfg.DiskPages) {
		for _, disk := range s.cfg.DiskPages[page-1] {
			if disk.Description != description || s.isDeleted(kindDisk, disk.Name) {
				continue
			}

			records += fmt.Sprintf(`  <DiskRecord href="%s/api/disk/%s" id="urn:vcloud:disk:%s" type="application/vnd.vmware.vcloud.disk+xml" name="%s" description="%s" sizeMb="8192" status="RESOLVED"/>`+"\n",
				s.url, disk.ID, disk.ID, disk.Name, disk.Description)
		}
	}

	nextPage := ""
	if page < len(s.cfg.DiskPages) {
		nextPage = fmt.Sprintf(`  <Link rel="nextPage" type="application/vnd.vmware.vcloud.query.records+xml" href="%s/api/query?type=disk&amp;page=%d"/>`+"\n",
			s.url, page+1)
	}

	writeXML(w, fmt.Sprintf(`<QueryResultRecords xmlns="http://www.vmware.com/vcloud/v1.5" type="application/vnd.vmware.vcloud.query.records+xml" name="disk" page="%d" pageSize="25">
%s%s</QueryResultRecords>`, page, nextPage, records))
}

// handleVdc answers with the vdc and the vApp that holds the vms.
func (s *Server) handleVdc(w http.ResponseWriter) {
	writeXML(w, fmt.Sprintf(`<Vdc xmlns="http://www.vmware.com/vcloud/v1.5" href="%[1]s/api/vdc/%[2]s" id="urn:vcloud:vdc:%[2]s" type="application/vnd.vmware.vcloud.vdc+xml" name="%[3]s" status="1">
  <AllocationModel>Flex</AllocationModel>
  <ResourceEntities>
    <ResourceEntity href="%[1]s/api/vApp/vapp-%[4]s" name="%[5]s" type="application/vnd.vmware.vcloud.vApp+xml"/>
  </ResourceEntities>
  <IsEnabled>true</IsEnabled>
</Vdc>`, s.url, vdcID, s.cfg.VdcName, vappID, s.cfg.VAppName))
}

// handleVApp lists every vm that a configured disk is attached to.
func (s *Server) handleVApp(w http.ResponseWriter) {
	children := ""
	for _, name := range s.attachedVMNames() {
		children += fmt.Sprintf(`    <Vm href="%s/api/vApp/vm-%s" id="urn:vcloud:vm:%s" type="application/vnd.vmware.vcloud.vm+xml" name="%s" status="4"/>`+"\n",
			s.url, name, name, name)
	}

	writeXML(w, fmt.Sprintf(`<VApp xmlns="http://www.vmware.com/vcloud/v1.5" href="%[1]s/api/vApp/vapp-%[2]s" id="urn:vcloud:vapp:%[2]s" type="application/vnd.vmware.vcloud.vApp+xml" name="%[3]s" status="4" deployed="true">
  <Children>
%[4]s  </Children>
</VApp>`, s.url, vappID, s.cfg.VAppName, children))
}

// handleVM answers with a vm, or performs a detach. govcd finds the detach
// endpoint through the link with rel "disk:detach", so the path is whatever
// this fake advertises.
func (s *Server) handleVM(w http.ResponseWriter, r *http.Request, rest string) {
	name, action, _ := strings.Cut(rest, "/")

	if action == "disk/action/detach" && r.Method == http.MethodPost {
		s.detachDisk(w, name)
		return
	}

	if action != "" {
		s.notImplemented(w, r)
		return
	}

	writeXML(w, fmt.Sprintf(`<Vm xmlns="http://www.vmware.com/vcloud/v1.5" href="%[1]s/api/vApp/vm-%[2]s" id="urn:vcloud:vm:%[2]s" type="application/vnd.vmware.vcloud.vm+xml" name="%[2]s" status="4" deployed="true">
  <Link rel="disk:attach" type="application/vnd.vmware.vcloud.diskAttachOrDetachParams+xml" href="%[1]s/api/vApp/vm-%[2]s/disk/action/attach"/>
  <Link rel="disk:detach" type="application/vnd.vmware.vcloud.diskAttachOrDetachParams+xml" href="%[1]s/api/vApp/vm-%[2]s/disk/action/detach"/>
</Vm>`, s.url, name))
}

// detachDisk releases every disk held by a vm. VCD publishes the remove link on
// a disk only after nothing is attached to it.
func (s *Server) detachDisk(w http.ResponseWriter, vmName string) {
	s.mu.Lock()
	for _, disk := range s.disks {
		if disk.AttachedVM == vmName {
			disk.AttachedVM = ""
			s.deletes[kindDetach] = append(s.deletes[kindDetach], disk.Name)
		}
	}
	s.mu.Unlock()

	s.writeTask(w)
}

// handleDisk serves a disk, its attached vms, or a delete.
func (s *Server) handleDisk(w http.ResponseWriter, r *http.Request, rest string) {
	id, action, _ := strings.Cut(rest, "/")

	s.mu.Lock()
	disk, ok := s.disks[id]
	s.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	switch {
	case action == "attachedVms":
		s.writeAttachedVMs(w, disk)
	case action != "":
		s.notImplemented(w, r)
	case r.Method == http.MethodDelete:
		if disk.AttachedVM != "" {
			w.WriteHeader(http.StatusConflict)
			return
		}
		s.recordDelete(kindDisk, disk.Name)
		s.writeTask(w)
	default:
		s.writeDisk(w, disk)
	}
}

func (s *Server) writeDisk(w http.ResponseWriter, disk *Disk) {
	links := fmt.Sprintf(`  <Link rel="down" type="application/vnd.vmware.vcloud.vms+xml" href="%s/api/disk/%s/attachedVms"/>`+"\n",
		s.url, disk.ID)

	// The remove link only exists while nothing holds the disk.
	if disk.AttachedVM == "" {
		links += fmt.Sprintf(`  <Link rel="remove" href="%s/api/disk/%s"/>`+"\n", s.url, disk.ID)
	}

	writeXML(w, fmt.Sprintf(`<Disk xmlns="http://www.vmware.com/vcloud/v1.5" href="%[1]s/api/disk/%[2]s" id="urn:vcloud:disk:%[2]s" type="application/vnd.vmware.vcloud.disk+xml" name="%[3]s" status="1" sizeMb="8192">
%[4]s  <Description>%[5]s</Description>
</Disk>`, s.url, disk.ID, disk.Name, links, disk.Description))
}

func (s *Server) writeAttachedVMs(w http.ResponseWriter, disk *Disk) {
	references := ""
	if disk.AttachedVM != "" {
		references = fmt.Sprintf(`  <VmReference href="%s/api/vApp/vm-%s" id="urn:vcloud:vm:%s" type="application/vnd.vmware.vcloud.vm+xml" name="%s"/>`+"\n",
			s.url, disk.AttachedVM, disk.AttachedVM, disk.AttachedVM)
	}

	writeXML(w, fmt.Sprintf(`<Vms xmlns="http://www.vmware.com/vcloud/v1.5" type="application/vnd.vmware.vcloud.vms+xml" href="%s/api/disk/%s/attachedVms">
%s</Vms>`, s.url, disk.ID, references))
}

func (s *Server) handleTask(w http.ResponseWriter) {
	s.writeTask(w)
}

// writeTask answers with a finished task. govcd always polls a task once, so a
// task that is already successful keeps the tests fast. A running task would
// cost a hard coded three second sleep per poll.
func (s *Server) writeTask(w http.ResponseWriter) {
	writeXML(w, fmt.Sprintf(`<Task xmlns="http://www.vmware.com/vcloud/v1.5" href="%[1]s/api/task/%[2]s" id="urn:vcloud:task:%[2]s" type="application/vnd.vmware.vcloud.task+xml" name="task" operation="fake" status="success"/>`,
		s.url, taskID))
}

// attachedVMNames lists the vms that still hold a disk, sorted for stable
// output.
func (s *Server) attachedVMNames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	seen := map[string]bool{}
	names := []string{}
	for _, page := range s.cfg.DiskPages {
		for _, disk := range page {
			if disk.AttachedVM == "" || seen[disk.AttachedVM] {
				continue
			}
			seen[disk.AttachedVM] = true
			names = append(names, disk.AttachedVM)
		}
	}

	return names
}

// isDeleted reports whether a name was already removed.
func (s *Server) isDeleted(kind, name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, deleted := range s.deletes[kind] {
		if deleted == name {
			return true
		}
	}

	return false
}
