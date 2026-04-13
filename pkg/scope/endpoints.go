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
//
// Region matching order:
//  1. Exact match on regionName
//  2. Wildcard: empty string "" (matches any region)
//  3. Default: if only one region is configured for the cloud, use it
//
// This allows endpoints.yaml to work even when the effective region
// (from identityRef or clouds.yaml) does not exactly match the key
// in endpoints.yaml, which is the most common misconfiguration.
func (e *EndpointOverrides) GetEndpoint(service, cloudName, regionName string) string {
	if e == nil {
		return ""
	}
	byRegion, ok := e.Clouds[cloudName]
	if !ok {
		return ""
	}

	// 1. Exact match on region
	if byService, ok := byRegion[regionName]; ok {
		if url := byService[service]; url != "" {
			return url
		}
	}

	// 2. Wildcard: empty-string region acts as a catch-all default
	if regionName != "" {
		if byService, ok := byRegion[""]; ok {
			if url := byService[service]; url != "" {
				return url
			}
		}
	}

	// 3. If only one region is configured, use it as the default
	if len(byRegion) == 1 {
		for _, byService := range byRegion {
			if url := byService[service]; url != "" {
				return url
			}
		}
	}

	return ""
}

// AvailableRegions returns the region keys configured for a given cloud,
// or nil if the cloud is not found.  Used for diagnostic logging.
func (e *EndpointOverrides) AvailableRegions(cloudName string) []string {
	if e == nil {
		return nil
	}
	byRegion, ok := e.Clouds[cloudName]
	if !ok {
		return nil
	}
	regions := make([]string, 0, len(byRegion))
	for r := range byRegion {
		regions = append(regions, r)
	}
	return regions
}
