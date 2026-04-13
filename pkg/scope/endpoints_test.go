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

import "testing"

func TestEndpointOverrides_GetEndpoint(t *testing.T) {
	t.Parallel()

	overrides := &EndpointOverrides{
		Clouds: map[string]map[string]map[string]string{
			"mycloud": {
				"RegionOne": {
					"compute": "https://nova.example.com/v2.1/",
					"network": "https://neutron.example.com/v2.0/",
				},
				"RegionTwo": {
					"compute": "https://nova-r2.example.com/v2.1/",
				},
			},
		},
	}

	cases := []struct {
		name       string
		service    string
		cloudName  string
		regionName string
		want       string
	}{
		// Configured overrides.
		{"exact match RegionOne compute", "compute", "mycloud", "RegionOne", "https://nova.example.com/v2.1/"},
		{"exact match RegionTwo compute", "compute", "mycloud", "RegionTwo", "https://nova-r2.example.com/v2.1/"},
		{"exact match RegionOne network", "network", "mycloud", "RegionOne", "https://neutron.example.com/v2.0/"},
		// Missing region with multiple regions → no fallback (ambiguous).
		{"missing region with multiple", "compute", "mycloud", "RegionThree", ""},
		// Missing cloud → empty string.
		{"missing cloud", "compute", "othercloud", "RegionOne", ""},
		// Service not configured at all → empty string.
		{"unconfigured volume service", "volume", "mycloud", "RegionOne", ""},
		{"unconfigured image service", "image", "mycloud", "RegionOne", ""},
		{"unconfigured loadbalancer service", "loadbalancer", "mycloud", "RegionOne", ""},
		// Unknown service name → empty string.
		{"unknown service", "unknown", "mycloud", "RegionOne", ""},
	}

	for _, tc := range cases {
		got := overrides.GetEndpoint(tc.service, tc.cloudName, tc.regionName)
		if got != tc.want {
			t.Errorf("%s: GetEndpoint(%q, %q, %q) = %q, want %q", tc.name, tc.service, tc.cloudName, tc.regionName, got, tc.want)
		}
	}
}

func TestEndpointOverrides_GetEndpoint_NilReceiver(t *testing.T) {
	t.Parallel()
	var overrides *EndpointOverrides
	if got := overrides.GetEndpoint("compute", "mycloud", "RegionOne"); got != "" {
		t.Errorf("expected empty string from nil receiver, got %q", got)
	}
}

// TestEndpointOverrides_GetEndpoint_SingleRegionFallback tests that when only
// one region is configured for a cloud, it is used as a default even when the
// requested region doesn't match exactly.
func TestEndpointOverrides_GetEndpoint_SingleRegionFallback(t *testing.T) {
	t.Parallel()

	overrides := &EndpointOverrides{
		Clouds: map[string]map[string]map[string]string{
			"mycloud": {
				"RegionOne": {
					"compute": "https://nova.example.com/v2.1/",
				},
			},
		},
	}

	cases := []struct {
		name       string
		regionName string
		want       string
	}{
		{"exact match", "RegionOne", "https://nova.example.com/v2.1/"},
		{"empty region falls back to single entry", "", "https://nova.example.com/v2.1/"},
		{"wrong region falls back to single entry", "OtherRegion", "https://nova.example.com/v2.1/"},
	}

	for _, tc := range cases {
		got := overrides.GetEndpoint("compute", "mycloud", tc.regionName)
		if got != tc.want {
			t.Errorf("%s: GetEndpoint(compute, mycloud, %q) = %q, want %q", tc.name, tc.regionName, got, tc.want)
		}
	}
}

// TestEndpointOverrides_GetEndpoint_WildcardRegion tests that an empty-string
// region key in endpoints.yaml acts as a wildcard that matches any region.
func TestEndpointOverrides_GetEndpoint_WildcardRegion(t *testing.T) {
	t.Parallel()

	overrides := &EndpointOverrides{
		Clouds: map[string]map[string]map[string]string{
			"mycloud": {
				"": {
					"compute": "https://nova-wildcard.example.com/v2.1/",
				},
				"RegionOne": {
					"compute": "https://nova-r1.example.com/v2.1/",
				},
			},
		},
	}

	cases := []struct {
		name       string
		regionName string
		want       string
	}{
		// Exact match takes priority.
		{"exact match RegionOne", "RegionOne", "https://nova-r1.example.com/v2.1/"},
		// Unknown region falls back to wildcard "".
		{"wildcard fallback", "SomeOtherRegion", "https://nova-wildcard.example.com/v2.1/"},
	}

	for _, tc := range cases {
		got := overrides.GetEndpoint("compute", "mycloud", tc.regionName)
		if got != tc.want {
			t.Errorf("%s: GetEndpoint(compute, mycloud, %q) = %q, want %q", tc.name, tc.regionName, got, tc.want)
		}
	}
}

// TestEndpointOverrides_AvailableRegions tests the diagnostic helper.
func TestEndpointOverrides_AvailableRegions(t *testing.T) {
	t.Parallel()

	overrides := &EndpointOverrides{
		Clouds: map[string]map[string]map[string]string{
			"mycloud": {
				"RegionOne": {"compute": "url1"},
				"RegionTwo": {"compute": "url2"},
			},
		},
	}

	regions := overrides.AvailableRegions("mycloud")
	if len(regions) != 2 {
		t.Fatalf("expected 2 regions, got %v", regions)
	}

	if got := overrides.AvailableRegions("missing"); got != nil {
		t.Errorf("expected nil for missing cloud, got %v", got)
	}

	var nilOverrides *EndpointOverrides
	if got := nilOverrides.AvailableRegions("mycloud"); got != nil {
		t.Errorf("expected nil from nil receiver, got %v", got)
	}
}
