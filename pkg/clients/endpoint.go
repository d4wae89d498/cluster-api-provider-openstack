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

package clients

import (
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"k8s.io/klog/v2"
)

// ApplyEndpointOverride applies an endpoint URL override to a gophercloud
// ServiceClient.  It handles three cases:
//
//  1. Catalog lookup succeeded AND an override URL is provided: replace both
//     Endpoint and ResourceBase so that ResourceBaseURL() returns the override.
//  2. Catalog lookup failed AND an override URL is provided: create a new
//     ServiceClient using the override URL directly, bypassing the catalog.
//  3. No override URL: return the catalog client as-is (or the original error
//     if the catalog lookup failed).
//
// The serviceName parameter is only used for log messages.
func ApplyEndpointOverride(serviceClient *gophercloud.ServiceClient, catalogErr error, providerClient *gophercloud.ProviderClient, endpointURL, serviceName string) (*gophercloud.ServiceClient, error) {
	if catalogErr != nil && endpointURL != "" {
		// Catalog lookup failed, but an explicit override is available.
		klog.V(4).Infof("New%sClient: catalog lookup failed, using override endpoint=%q", serviceName, endpointURL)
		return &gophercloud.ServiceClient{
			ProviderClient: providerClient,
			Endpoint:       endpointURL,
		}, nil
	}

	if catalogErr != nil {
		return nil, fmt.Errorf("failed to create %s service client: %v", serviceName, catalogErr)
	}

	if endpointURL != "" {
		klog.V(4).Infof("New%sClient: overriding catalog endpoint=%q resourceBase=%q with=%q",
			serviceName, serviceClient.Endpoint, serviceClient.ResourceBase, endpointURL)
		// Override both Endpoint and ResourceBase.  Several gophercloud
		// New*() helpers set ResourceBase to Endpoint + version-path
		// (e.g. "v2/", "v2.0/").  If we only override Endpoint, the
		// stale ResourceBase still wins in ResourceBaseURL().  Clearing
		// it forces ResourceBaseURL() to fall back to the new Endpoint.
		serviceClient.Endpoint = endpointURL
		serviceClient.ResourceBase = ""
		return serviceClient, nil
	}

	klog.V(4).Infof("New%sClient: using catalog endpoint=%q resourceBase=%q",
		serviceName, serviceClient.Endpoint, serviceClient.ResourceBase)
	return serviceClient, nil
}
