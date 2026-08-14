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

// Package vcdfake serves a subset of the VMware Cloud Director rest api. It is
// large enough for the govcd and vcdsdk clients to authenticate and for the
// cleaners to list and delete resources.
//
// The server keeps state, so a delete really removes the object from the next
// listing. Unknown routes answer 501 and are recorded, which names the missing
// endpoint when a test fails.
package vcdfake

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
)

// Fixed identifiers. Real VCD uses random uuids, the values only need to be
// stable and distinct.
const (
	orgID     = "11111111-1111-1111-1111-111111111111"
	vdcID     = "22222222-2222-2222-2222-222222222222"
	vappID    = "33333333-3333-3333-3333-333333333333"
	networkID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	gatewayID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	taskID    = "cccccccc-cccc-cccc-cccc-cccccccccccc"

	orgURN     = "urn:vcloud:org:" + orgID
	networkURN = "urn:vcloud:network:" + networkID

	// GatewayURN is the edge gateway the fake network is attached to.
	GatewayURN = "urn:vcloud:gateway:" + gatewayID

	// accessToken must be longer than 32 characters, otherwise govcd omits the
	// bearer Authorization header that the swagger client relies on.
	accessToken = "fake-vcd-access-token-0123456789abcdef"

	apiVersion = "36.0"
)

// Disk is a named disk in the fake VCD.
type Disk struct {
	ID          string
	Name        string
	Description string

	// AttachedVM is the name of the vm holding the disk. VCD publishes the
	// remove link only once nothing is attached, so an attached disk must be
	// detached before it can be deleted.
	AttachedVM string
}

// Resource is any VCD object the cleaners match by name.
type Resource struct {
	ID   string
	Name string
}

// Config describes the state the server starts with.
type Config struct {
	OrgName     string
	VdcName     string
	NetworkName string
	Username    string
	Password    string

	// VAppName holds the vms. The volume cleaner looks vms up in the vApp named
	// after the VCDCluster.
	VAppName string

	// DiskPages is one entry per page of the disk query.
	DiskPages [][]Disk

	// NatRules are the edge gateway nat rules.
	NatRules []Resource
	// NatRulePageSize is how many nat rules one cursor page holds. Zero puts
	// them all on a single page.
	NatRulePageSize int

	VirtualServices []Resource
	Pools           []Resource
	AppPortProfiles []Resource
}

// Server is a fake VCD endpoint.
type Server struct {
	server *httptest.Server
	cfg    Config

	mu        sync.Mutex
	url       string
	disks     map[string]*Disk
	natRules  map[string]string
	resources map[string]map[string]string

	requests  []string
	deletes   map[string][]string
	unhandled []string

	loginUser     string
	loginPassword string
}

// New starts a fake VCD server. The caller must close it.
func New(cfg Config) *Server {
	s := &Server{
		cfg:       cfg,
		disks:     map[string]*Disk{},
		natRules:  map[string]string{},
		resources: map[string]map[string]string{},
		deletes:   map[string][]string{},
	}

	for _, page := range cfg.DiskPages {
		for _, disk := range page {
			d := disk
			s.disks[d.ID] = &d
		}
	}
	for _, rule := range cfg.NatRules {
		s.natRules[rule.ID] = rule.Name
	}
	s.resources[kindVirtualService] = resourceMap(cfg.VirtualServices)
	s.resources[kindPool] = resourceMap(cfg.Pools)
	s.resources[kindAppPortProfile] = resourceMap(cfg.AppPortProfiles)

	s.server = httptest.NewServer(http.HandlerFunc(s.route))
	s.url = s.server.URL

	return s
}

// URL is the site address to put into VCDCluster.Spec.Site.
func (s *Server) URL() string {
	return s.server.URL
}

// Close shuts the server down.
func (s *Server) Close() {
	s.server.Close()
}

const (
	kindVirtualService = "virtualService"
	kindPool           = "pool"
	kindAppPortProfile = "appPortProfile"
	kindDisk           = "disk"
	kindNatRule        = "natRule"
	kindDetach         = "detach"
)

func resourceMap(resources []Resource) map[string]string {
	out := map[string]string{}
	for _, r := range resources {
		out[r.ID] = r.Name
	}

	return out
}

// route dispatches on method and path. It uses plain string matching because
// VCD puts urns, which contain colons, into path segments.
func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	record := r.Method + " " + r.URL.Path
	if r.URL.RawQuery != "" {
		record += "?" + r.URL.RawQuery
	}

	s.mu.Lock()
	s.requests = append(s.requests, record)
	s.mu.Unlock()

	path := r.URL.Path

	switch {
	case path == "/api/versions":
		s.handleVersions(w)
	case strings.HasPrefix(path, "/cloudapi/1.0.0/sessions"):
		s.handleSession(w, r)
	case path == "/api/org" || path == "/api/org/":
		s.handleOrgList(w)
	case path == "/api/org/"+orgID:
		s.handleOrg(w)
	case path == "/api/query":
		s.handleQuery(w, r)
	case path == "/api/vdc/"+vdcID:
		s.handleVdc(w)
	case path == "/api/vApp/vapp-"+vappID:
		s.handleVApp(w)
	case strings.HasPrefix(path, "/api/vApp/vm-"):
		s.handleVM(w, r, strings.TrimPrefix(path, "/api/vApp/vm-"))
	case strings.HasPrefix(path, "/api/disk/"):
		s.handleDisk(w, r, strings.TrimPrefix(path, "/api/disk/"))
	case strings.HasPrefix(path, "/api/task/"):
		s.handleTask(w)
	case strings.HasPrefix(path, "/cloudapi/"):
		s.routeCloudAPI(w, r, strings.TrimPrefix(path, "/cloudapi/1.0.0"))
	default:
		s.notImplemented(w, r)
	}
}

func (s *Server) notImplemented(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.unhandled = append(s.unhandled, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	s.mu.Unlock()

	w.WriteHeader(http.StatusNotImplemented)
	_, _ = fmt.Fprintf(w, "vcdfake has no handler for %s %s", r.Method, r.URL.Path) //nolint:gosec // test double, the body is not a web page
}

// recordDelete notes a deletion so a test can assert on it.
func (s *Server) recordDelete(kind, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.deletes[kind] = append(s.deletes[kind], name)
}

// Deleted lists the names deleted for a kind, in order.
func (s *Server) Deleted(kind string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.deletes[kind]...)
}

// DeletedDisks lists the disks that were deleted.
func (s *Server) DeletedDisks() []string { return s.Deleted(kindDisk) }

// DeletedNatRules lists the nat rules that were deleted.
func (s *Server) DeletedNatRules() []string { return s.Deleted(kindNatRule) }

// DeletedVirtualServices lists the virtual services that were deleted.
func (s *Server) DeletedVirtualServices() []string { return s.Deleted(kindVirtualService) }

// DeletedPools lists the load balancer pools that were deleted.
func (s *Server) DeletedPools() []string { return s.Deleted(kindPool) }

// DeletedAppPortProfiles lists the app port profiles that were deleted.
func (s *Server) DeletedAppPortProfiles() []string { return s.Deleted(kindAppPortProfile) }

// DetachedDisks lists the disks that were detached from a vm.
func (s *Server) DetachedDisks() []string { return s.Deleted(kindDetach) }

// Requests lists every request the server received, as "METHOD /path".
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.requests...)
}

// Unhandled lists the requests that hit no route. A non empty result means the
// fake is missing an endpoint.
func (s *Server) Unhandled() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]string(nil), s.unhandled...)
}

// Credentials returns the user name and password of the last login.
func (s *Server) Credentials() (string, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loginUser, s.loginPassword
}

// rawParam reads a query parameter straight from the raw query string.
// url.Values drops any parameter containing a semicolon, and VCD filters use
// semicolons to join conditions.
func rawParam(r *http.Request, key string) string {
	for _, part := range strings.Split(r.URL.RawQuery, "&") {
		name, value, found := strings.Cut(part, "=")
		if !found || name != key {
			continue
		}

		decoded, err := url.QueryUnescape(value)
		if err != nil {
			return value
		}

		return decoded
	}

	return ""
}

// filterName reads the name a VCD filter selects on, if it has one. Filters
// look like `name==foo` or `scope==TENANT;name==foo`.
func filterName(r *http.Request) string {
	for _, condition := range strings.Split(rawParam(r, "filter"), ";") {
		field, value, found := strings.Cut(condition, "==")
		if found && field == "name" {
			return value
		}
	}

	return ""
}

const xmlHeader = `<?xml version="1.0" encoding="UTF-8"?>` + "\n"

func writeXML(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/xml")
	_, _ = fmt.Fprint(w, xmlHeader+body) //nolint:gosec // test double, the body is a fixed fixture
}

// writeJSON answers with json. The swagger client refuses any response whose
// content type is not json or xml.
func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, body) //nolint:gosec // test double, the body is a fixed fixture
}
