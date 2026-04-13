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
		service    string
		cloudName  string
		regionName string
		want       string
	}{
		{"compute", "mycloud", "RegionOne", "https://nova.example.com/v2.1/"},
		{"compute", "mycloud", "RegionTwo", "https://nova-r2.example.com/v2.1/"},
		{"network", "mycloud", "RegionOne", "https://neutron.example.com/v2.0/"},
		// network not set for RegionTwo
		{"network", "mycloud", "RegionTwo", ""},
		// unknown service
		{"image", "mycloud", "RegionOne", ""},
		// unknown cloud
		{"compute", "othercloud", "RegionOne", ""},
		// unknown region
		{"compute", "mycloud", "RegionThree", ""},
	}

	for _, tc := range cases {
		got := overrides.GetEndpoint(tc.service, tc.cloudName, tc.regionName)
		if got != tc.want {
			t.Errorf("GetEndpoint(%q, %q, %q) = %q, want %q",
				tc.service, tc.cloudName, tc.regionName, got, tc.want)
		}
	}
}

func TestEndpointOverrides_GetEndpoint_Nil(t *testing.T) {
	t.Parallel()
	var e *EndpointOverrides
	if got := e.GetEndpoint("compute", "mycloud", "RegionOne"); got != "" {
		t.Errorf("nil receiver: expected empty string, got %q", got)
	}
}
