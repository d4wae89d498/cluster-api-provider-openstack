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
//	<service>:
//	  <cloudName>:
//	    <regionName>: <url>
//
// Supported services: compute, network, volume, image, loadbalancer.
// All endpoint URLs MUST end with a trailing slash ('/').
//
// Example:
//
//	compute:
//	  mycloud:
//	    RegionOne: https://nova-custom.example.com/v2.1/
//	    RegionTwo: https://nova-regiontwo.example.com/v2.1/
//	network:
//	  mycloud:
//	    RegionOne: https://neutron-custom.example.com/v2.0/
//	volume:
//	  mycloud:
//	    RegionOne: https://cinder-custom.example.com/v3/
//	image:
//	  mycloud:
//	    RegionOne: https://glance-custom.example.com/v2/
//	loadbalancer:
//	  mycloud:
//	    RegionOne: https://octavia-custom.example.com/v2.0/
type EndpointOverrides struct {
	// Compute holds endpoint overrides for the Nova (compute) service.
	// +optional
	Compute map[string]map[string]string `json:"compute,omitempty" yaml:"compute,omitempty"`

	// Network holds endpoint overrides for the Neutron (network) service.
	// +optional
	Network map[string]map[string]string `json:"network,omitempty" yaml:"network,omitempty"`

	// Volume holds endpoint overrides for the Cinder (block storage) service.
	// +optional
	Volume map[string]map[string]string `json:"volume,omitempty" yaml:"volume,omitempty"`

	// Image holds endpoint overrides for the Glance (image) service.
	// +optional
	Image map[string]map[string]string `json:"image,omitempty" yaml:"image,omitempty"`

	// LoadBalancer holds endpoint overrides for the Octavia (load balancer) service.
	// +optional
	LoadBalancer map[string]map[string]string `json:"loadbalancer,omitempty" yaml:"loadbalancer,omitempty"`
}

// GetEndpoint returns the custom endpoint URL for the given service, cloud name,
// and region name.  It returns an empty string when no override is configured
// for the requested combination.
func (e *EndpointOverrides) GetEndpoint(service, cloudName, regionName string) string {
	if e == nil {
		return ""
	}
	var byCloud map[string]map[string]string
	switch service {
	case "compute":
		byCloud = e.Compute
	case "network":
		byCloud = e.Network
	case "volume":
		byCloud = e.Volume
	case "image":
		byCloud = e.Image
	case "loadbalancer":
		byCloud = e.LoadBalancer
	default:
		return ""
	}
	byRegion, ok := byCloud[cloudName]
	if !ok {
		return ""
	}
	return byRegion[regionName]
}
