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
	"errors"
	"testing"

	"github.com/gophercloud/gophercloud/v2"
)

func TestApplyEndpointOverride_NoOverride_CatalogOK(t *testing.T) {
	t.Parallel()
	sc := &gophercloud.ServiceClient{
		Endpoint:     "https://catalog.example.com/v2/",
		ResourceBase: "https://catalog.example.com/v2/",
	}
	got, err := ApplyEndpointOverride(sc, nil, nil, "", "Test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Endpoint != "https://catalog.example.com/v2/" {
		t.Errorf("Endpoint = %q, want catalog value", got.Endpoint)
	}
	if got.ResourceBase != "https://catalog.example.com/v2/" {
		t.Errorf("ResourceBase should be unchanged, got %q", got.ResourceBase)
	}
}

func TestApplyEndpointOverride_NoOverride_CatalogFails(t *testing.T) {
	t.Parallel()
	_, err := ApplyEndpointOverride(nil, errors.New("no endpoint"), nil, "", "Test")
	if err == nil {
		t.Fatal("expected error when catalog fails and no override")
	}
}

func TestApplyEndpointOverride_WithOverride_CatalogOK(t *testing.T) {
	t.Parallel()
	sc := &gophercloud.ServiceClient{
		Endpoint:     "https://catalog.example.com/v2/",
		ResourceBase: "https://catalog.example.com/v2/subpath/",
	}
	got, err := ApplyEndpointOverride(sc, nil, nil, "https://custom.example.com/v2/", "Test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Endpoint != "https://custom.example.com/v2/" {
		t.Errorf("Endpoint = %q, want override value", got.Endpoint)
	}
	if got.ResourceBase != "" {
		t.Errorf("ResourceBase should be cleared, got %q", got.ResourceBase)
	}
}

func TestApplyEndpointOverride_WithOverride_CatalogFails(t *testing.T) {
	t.Parallel()
	pc := &gophercloud.ProviderClient{}
	got, err := ApplyEndpointOverride(nil, errors.New("no endpoint"), pc, "https://custom.example.com/v2/", "Test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Endpoint != "https://custom.example.com/v2/" {
		t.Errorf("Endpoint = %q, want override value", got.Endpoint)
	}
	if got.ProviderClient != pc {
		t.Error("ProviderClient should be from the argument")
	}
}
