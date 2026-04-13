/*
Copyright 2026 The Kubernetes Authors.

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

package scope

// EndpointsSecretKey is the key in the credentials secret that optionally contains
// per-service endpoint overrides.  The value must be a YAML-encoded EndpointOverrides.
const EndpointsSecretKey = "endpoints.yaml"

// EndpointOverrides allows customizing OpenStack service endpoint URLs on a
// per-cloud, per-region basis.  It is loaded from the "endpoints.yaml" key in
// the credentials secret.
//
// Structure:
//
//	clouds:
//	  <cloudName>:
//	    <regionName>:
//	      <service>: <url>
//
// Supported services: compute, network, volume, image, loadbalancer.
// All endpoint URLs MUST end with a trailing slash ('/').
//
// Example:
//
//	clouds:
//	  mycloud:
//	    RegionOne:
//	      compute: https://nova-custom.example.com/v2.1/
//	      network: https://neutron-custom.example.com/v2.0/
//	    RegionTwo:
//	      compute: https://nova-regiontwo.example.com/v2.1/
type EndpointOverrides struct {
	// Clouds holds per-cloud, per-region endpoint overrides.
	// The outer key is the cloud name (as it appears in clouds.yaml), the
	// middle key is the region name, and the inner key is the service name
	// (one of: compute, network, volume, image, loadbalancer).
	// +optional
	Clouds map[string]map[string]map[string]string `json:"clouds,omitempty" yaml:"clouds,omitempty"`
}

// GetEndpoint returns the custom endpoint URL for the given service, cloud name,
// and region name.  It returns an empty string when no override is configured
// for the requested combination.
func (e *EndpointOverrides) GetEndpoint(service, cloudName, regionName string) string {
	if e == nil {
		return ""
	}
	byRegion, ok := e.Clouds[cloudName]
	if !ok {
		return ""
	}
	byService, ok := byRegion[regionName]
	if !ok {
		return ""
	}
	return byService[service]
}
