/*
Copyright 2019 The Kubernetes Authors.

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

package main

import (
	"encoding/json"
	"fmt"

	extensions "k8s.io/api/extensions/v1beta1"
	networking "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

// hasResource determines if an API resource is available.
func hasResource(client discovery.DiscoveryInterface, resource schema.GroupVersionResource) bool {
	resources, err := client.ServerResourcesForGroupVersion(resource.GroupVersion().String())
	if err != nil {
		return false
	}

	for _, serverResource := range resources.APIResources {
		if serverResource.Name == resource.Resource {
			return true
		}
	}

	return false
}

// toNewIngress converts a legacy "extensions/v1beta1" IngressList to the newer "networking.k8s.io/v1beta1" IngressList.
func toNewIngress(oldIng *extensions.IngressList) (*networking.IngressList, error) {

	raw, err := json.Marshal(oldIng)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal old ingress: %w", err)
	}

	var newIng networking.IngressList
	if err := json.Unmarshal(raw, &newIng); err != nil {
		return nil, fmt.Errorf("failed to unmarshal new ingress: %w", err)
	}

	return &newIng, err
}
