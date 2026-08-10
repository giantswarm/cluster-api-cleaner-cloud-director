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

// routeCloudAPI dispatches the json api. path has the /cloudapi/1.0.0 prefix
// removed.
func (s *Server) routeCloudAPI(w http.ResponseWriter, r *http.Request, path string) {
	switch {
	case path == "/orgVdcNetworks":
		s.handleNetworkList(w, r)
	case path == "/orgVdcNetworks/"+networkURN:
		s.handleNetwork(w)
	case strings.HasPrefix(path, "/edgeGateways/"+GatewayURN):
		s.routeEdgeGateway(w, r, strings.TrimPrefix(path, "/edgeGateways/"+GatewayURN))
	case strings.HasPrefix(path, "/loadBalancer/virtualServices/"):
		s.handleResource(w, r, kindVirtualService, strings.TrimPrefix(path, "/loadBalancer/virtualServices/"))
	case strings.HasPrefix(path, "/loadBalancer/pools/"):
		s.handleResource(w, r, kindPool, strings.TrimPrefix(path, "/loadBalancer/pools/"))
	case path == "/applicationPortProfiles", path == "/applicationPortProfiles/":
		// govcd asks for the collection with a trailing slash.
		s.handleAppPortProfileList(w, r)
	case strings.HasPrefix(path, "/applicationPortProfiles/"):
		s.handleResource(w, r, kindAppPortProfile, strings.TrimPrefix(path, "/applicationPortProfiles/"))
	default:
		s.notImplemented(w, r)
	}
}

func (s *Server) routeEdgeGateway(w http.ResponseWriter, r *http.Request, path string) {
	switch {
	case path == "":
		s.handleEdgeGateway(w)
	case path == "/nat/rules":
		s.handleNatRuleList(w, r)
	case strings.HasPrefix(path, "/nat/rules/"):
		s.handleNatRuleDelete(w, r, strings.TrimPrefix(path, "/nat/rules/"))
	case path == "/loadBalancer/virtualServiceSummaries":
		s.handleSummaries(w, r, kindVirtualService)
	case path == "/loadBalancer/poolSummaries":
		s.handleSummaries(w, r, kindPool)
	default:
		s.notImplemented(w, r)
	}
}

// handleNetworkList lists the org vdc networks. vcdsdk walks the pages until it
// gets an empty one, so page two must be empty or it loops for ever.
func (s *Server) handleNetworkList(w http.ResponseWriter, r *http.Request) {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}

	if page > 1 {
		writeJSON(w, `{"resultTotal":1,"pageCount":1,"page":2,"pageSize":32,"values":[]}`)
		return
	}

	writeJSON(w, fmt.Sprintf(`{"resultTotal":1,"pageCount":1,"page":1,"pageSize":32,"values":[{"id":%q,"name":%q}]}`,
		networkURN, s.cfg.NetworkName))
}

// handleNetwork answers with the network and the edge gateway behind it. The
// cleaners fail with a nil gateway reference if connection.routerRef is absent.
func (s *Server) handleNetwork(w http.ResponseWriter) {
	writeJSON(w, fmt.Sprintf(`{
  "id": %q,
  "name": %q,
  "backingNetworkType": "NSXT_FLEXIBLE_SEGMENT",
  "connection": {
    "routerRef": {"name": "fake-edge", "id": %q},
    "connectionType": "INTERNAL",
    "connected": true
  }
}`, networkURN, s.cfg.NetworkName, GatewayURN))
}

// handleEdgeGateway reports the gateway as realized. vcdsdk dereferences the
// status without a nil check, so it must always be present.
func (s *Server) handleEdgeGateway(w http.ResponseWriter) {
	writeJSON(w, fmt.Sprintf(`{"id":%q,"name":"fake-edge","status":"REALIZED"}`, GatewayURN))
}

// handleNatRuleList returns one cursor page of nat rules. Paging runs over the
// rules that still exist, the way VCD does it, so a rule never falls into a
// hole left by an earlier delete.
//
// The Link header must have no space after the closing angle bracket, otherwise
// the cursor parser silently returns nothing and the listing stops early.
func (s *Server) handleNatRuleList(w http.ResponseWriter, r *http.Request) {
	page, err := strconv.Atoi(r.URL.Query().Get("cursor"))
	if err != nil || page < 1 {
		page = 1
	}

	live := s.liveNatRules()

	size := s.cfg.NatRulePageSize
	if size < 1 {
		size = len(live)
	}
	if size < 1 {
		size = 1
	}

	start := (page - 1) * size
	end := start + size
	if start > len(live) {
		start = len(live)
	}
	if end > len(live) {
		end = len(live)
	}

	values := []string{}
	for _, rule := range live[start:end] {
		values = append(values, fmt.Sprintf(`{"id":%q,"name":%q,"type":"DNAT","enabled":true}`, rule.ID, rule.Name))
	}

	if end < len(live) {
		w.Header().Set("Link", fmt.Sprintf(`<%s/cloudapi/1.0.0/edgeGateways/%s/nat/rules?cursor=%d&pageSize=128>;rel="nextPage";type="application/json"`,
			s.url, GatewayURN, page+1))
	}

	writeJSON(w, fmt.Sprintf(`{"status":"REALIZED","values":[%s]}`, strings.Join(values, ",")))
}

// liveNatRules lists the rules that are not deleted yet.
func (s *Server) liveNatRules() []Resource {
	live := []Resource{}
	for _, rule := range s.cfg.NatRules {
		if !s.isDeleted(kindNatRule, rule.Name) {
			live = append(live, rule)
		}
	}

	return live
}

// handleNatRuleDelete removes a nat rule. The swagger client requires 202 with
// a Location header pointing at a task.
func (s *Server) handleNatRuleDelete(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodDelete {
		s.notImplemented(w, r)
		return
	}

	s.mu.Lock()
	name, ok := s.natRules[id]
	s.mu.Unlock()

	if !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	s.recordDelete(kindNatRule, name)
	s.acceptTask(w)
}

// handleSummaries lists virtual services or load balancer pools. vcdsdk looks
// an object up by name and treats anything other than exactly one match as not
// found, so the name filter has to be honoured.
func (s *Server) handleSummaries(w http.ResponseWriter, r *http.Request, kind string) {
	wanted := filterName(r)

	values := []string{}
	for _, resource := range s.liveResources(kind) {
		if wanted != "" && resource.Name != wanted {
			continue
		}

		values = append(values, fmt.Sprintf(`{"id":%q,"name":%q,"status":"REALIZED","healthStatus":"UP","enabled":true,"virtualIpAddress":"10.0.0.5","gatewayRef":{"name":"fake-edge","id":%q}}`,
			resource.ID, resource.Name, GatewayURN))
	}

	s.writePages(w, values)
}

// handleAppPortProfileList lists the tenant scoped app port profiles.
func (s *Server) handleAppPortProfileList(w http.ResponseWriter, r *http.Request) {
	wanted := filterName(r)

	values := []string{}
	for _, resource := range s.liveResources(kindAppPortProfile) {
		if wanted != "" && resource.Name != wanted {
			continue
		}

		values = append(values, s.appPortProfileJSON(resource))
	}

	s.writePages(w, values)
}

// handleResource serves a single object by id, or deletes it.
func (s *Server) handleResource(w http.ResponseWriter, r *http.Request, kind, id string) {
	s.mu.Lock()
	name, ok := s.resources[kind][id]
	s.mu.Unlock()

	if !ok || s.isDeleted(kind, name) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	if r.Method == http.MethodDelete {
		s.recordDelete(kind, name)

		// govcd finishes on 204, the swagger client insists on 202 plus a task.
		if kind == kindAppPortProfile {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		s.acceptTask(w)

		return
	}

	if kind == kindAppPortProfile {
		writeJSON(w, s.appPortProfileJSON(Resource{ID: id, Name: name}))
		return
	}

	writeJSON(w, fmt.Sprintf(`{"id":%q,"name":%q,"status":"REALIZED","healthStatus":"UP","enabled":true,"virtualIpAddress":"10.0.0.5","gatewayRef":{"name":"fake-edge","id":%q}}`,
		id, name, GatewayURN))
}

func (s *Server) appPortProfileJSON(resource Resource) string {
	return fmt.Sprintf(`{"id":%q,"name":%q,"scope":"TENANT","applicationPorts":[{"protocol":"TCP","destinationPorts":["443"]}],"orgRef":{"name":%q,"id":%q},"contextEntityId":"urn:vcloud:vdc:%s"}`,
		resource.ID, resource.Name, s.cfg.OrgName, orgURN, vdcID)
}

// writePages wraps values in the openapi page envelope. resultTotal must not
// exceed pageSize, otherwise govcd asks for another page.
func (s *Server) writePages(w http.ResponseWriter, values []string) {
	writeJSON(w, fmt.Sprintf(`{"resultTotal":%d,"pageCount":1,"page":1,"pageSize":128,"values":[%s]}`,
		len(values), strings.Join(values, ",")))
}

// acceptTask answers a delete with 202 and a task location.
func (s *Server) acceptTask(w http.ResponseWriter) {
	w.Header().Set("Location", fmt.Sprintf("%s/api/task/%s", s.url, taskID))
	w.WriteHeader(http.StatusAccepted)
}

// liveResources lists the objects of a kind that are not deleted yet, in the
// order they were configured.
func (s *Server) liveResources(kind string) []Resource {
	var configured []Resource
	switch kind {
	case kindVirtualService:
		configured = s.cfg.VirtualServices
	case kindPool:
		configured = s.cfg.Pools
	case kindAppPortProfile:
		configured = s.cfg.AppPortProfiles
	}

	live := []Resource{}
	for _, resource := range configured {
		if !s.isDeleted(kind, resource.Name) {
			live = append(live, resource)
		}
	}

	return live
}
