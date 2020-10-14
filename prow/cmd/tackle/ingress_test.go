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
	"bytes"
	"testing"
	"time"

	extensions "k8s.io/api/extensions/v1beta1"
	networking "k8s.io/api/networking/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	discoveryFake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes"
	k8sFake "k8s.io/client-go/kubernetes/fake"
)

func createExtensionsIngressList() *extensions.IngressList {
	return &extensions.IngressList{
		Items: []extensions.Ingress{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "old-ingress",
					Namespace:         "demo",
					CreationTimestamp: metav1.NewTime(time.Now()),
				},
				Spec: extensions.IngressSpec{
					Rules: []extensions.IngressRule{
						{
							Host: "foo.bar",
							IngressRuleValue: extensions.IngressRuleValue{
								HTTP: &extensions.HTTPIngressRuleValue{
									Paths: []extensions.HTTPIngressPath{
										{
											Backend: extensions.IngressBackend{
												ServiceName: "demo",
												ServicePort: intstr.FromInt(80),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func createNetworkingIngressList() *networking.IngressList {
	return &networking.IngressList{
		Items: []networking.Ingress{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "old-ingress",
					Namespace:         "demo",
					CreationTimestamp: metav1.NewTime(time.Now()),
				},
				Spec: networking.IngressSpec{
					Rules: []networking.IngressRule{
						{
							Host: "foo.bar",
							IngressRuleValue: networking.IngressRuleValue{
								HTTP: &networking.HTTPIngressRuleValue{
									Paths: []networking.HTTPIngressPath{
										{
											Backend: networking.IngressBackend{
												ServiceName: "demo",
												ServicePort: intstr.FromInt(80),
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestHasResource(t *testing.T) {
	tests := []struct {
		name         string
		createClient func() kubernetes.Interface
		expected     bool
	}{
		{
			name: "networking ingress is unavailable",
			createClient: func() kubernetes.Interface {
				return k8sFake.NewSimpleClientset()
			},
			expected: false,
		},
		{
			name: "networking ingress is available",
			createClient: func() kubernetes.Interface {
				fakeClient := k8sFake.NewSimpleClientset()
				fakeDiscovery := fakeClient.Discovery().(*discoveryFake.FakeDiscovery)
				fakeNetworking := metav1.APIResourceList{
					GroupVersion: "networking.k8s.io/v1beta1",
					APIResources: []metav1.APIResource{{Name: "ingresses"}},
				}
				fakeDiscovery.Resources = append(fakeDiscovery.Resources, &fakeNetworking)
				return fakeClient
			},
			expected: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := test.createClient()
			isAvailable := hasResource(client.Discovery(), networking.SchemeGroupVersion.WithResource("ingresses"))

			if isAvailable != test.expected {
				t.Errorf("Expected %v, but got result %v", test.expected, isAvailable)
			}
		})
	}
}

func TestToNewIngress(t *testing.T) {
	oldIng, err := toNewIngress(createExtensionsIngressList())
	if err != nil {
		t.Errorf("Unexpected error converting extensions ingress: %v", err)
	}

	oldBytes, err := oldIng.Marshal()
	if err != nil {
		t.Errorf("Unexpected error marshalling extensions ingress: %v", err)
	}

	newIng := createNetworkingIngressList()

	newBytes, err := newIng.Marshal()
	if err != nil {
		t.Errorf("Unexpected error marshalling networking ingress: %v", err)
	}

	if !bytes.Equal(oldBytes, newBytes) {
		t.Errorf("Expected marshalling of types should be equal")
	}
}
