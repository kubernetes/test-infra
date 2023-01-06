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

package certificate

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	k8sFake "k8s.io/client-go/kubernetes/fake"
)

func TestCreateClusterCertificateCredentials(t *testing.T) {
	tests := []struct {
		name         string
		createClient func() kubernetes.Interface
		expected     bool
	}{
		{
			name: "create cluster certificate success",
			createClient: func() kubernetes.Interface {
				return k8sFake.NewSimpleClientset(&corev1.Secret{
					TypeMeta: metav1.TypeMeta{},
					ObjectMeta: metav1.ObjectMeta{
						Namespace: metav1.NamespaceSystem,
					},
					Data: map[string][]uint8{corev1.ServiceAccountRootCAKey: {1, 2, 3}},
				})
			},
			expected: true,
		},
		{
			name: "create cluster certificate fail",
			createClient: func() kubernetes.Interface {
				return k8sFake.NewSimpleClientset()
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := test.createClient()
			_, _, _, err := CreateClusterCertificateCredentials(client)
			success := err == nil

			if success != test.expected {
				t.Fatalf("Expected %v, but got result %v: %v", test.expected, success, err)
			}
		})
	}
}
