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

package serviceaccount

import (
	"testing"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	k8sFake "k8s.io/client-go/kubernetes/fake"
	k8sTesting "k8s.io/client-go/testing"
)

func TestCreateClusterServiceAccountCredentials(t *testing.T) {
	tests := []struct {
		name         string
		createClient func() kubernetes.Interface
		expected     bool
	}{
		{
			name: "create cluster service account success",
			createClient: func() kubernetes.Interface {
				var client kubernetes.Interface = &k8sFake.Clientset{}

				client.(*k8sFake.Clientset).Fake.AddReactor("get", "configmaps", func(action k8sTesting.Action) (handled bool, ret runtime.Object, err error) {
					r := &corev1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "kube-root-ca.crt",
							Namespace: corev1.NamespaceDefault,
						},
						Data: map[string]string{
							"ca.crt": "ca",
						},
					}
					return true, r, nil
				})

				client.(*k8sFake.Clientset).Fake.AddReactor("get", "serviceaccounts", func(action k8sTesting.Action) (handled bool, ret runtime.Object, err error) {
					r := &corev1.ServiceAccount{
						ObjectMeta: metav1.ObjectMeta{
							Name:      serviceAccountName,
							Namespace: corev1.NamespaceDefault,
						},
						Secrets: []corev1.ObjectReference{{Name: "secret-abc"}},
					}
					return true, r, nil
				})

				client.(*k8sFake.Clientset).Fake.AddReactor("create", "selfsubjectaccessreviews", func(action k8sTesting.Action) (handled bool, ret runtime.Object, err error) {
					r := &authorizationv1.SelfSubjectAccessReview{
						Status: authorizationv1.SubjectAccessReviewStatus{
							Allowed: true,
							Reason:  "I am a test!",
						},
					}
					return true, r, nil
				})

				client.(*k8sFake.Clientset).Fake.AddReactor("create", "serviceaccounts/token", func(action k8sTesting.Action) (handled bool, ret runtime.Object, err error) {
					r := &authenticationv1.TokenRequest{
						Status: authenticationv1.TokenRequestStatus{
							Token: "abc",
						},
					}
					return true, r, nil
				})

				return client
			},
			expected: true,
		},
		{
			name: "create cluster service account fail",
			createClient: func() kubernetes.Interface {
				return k8sFake.NewSimpleClientset()
			},
			expected: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := test.createClient()
			_, _, err := CreateClusterServiceAccountCredentials(client, metav1.Duration{Duration: 2 * 24 * time.Hour})
			success := err == nil

			if success != test.expected {
				t.Fatalf("Expected %v, but got result %v: %v", test.expected, success, err)
			}
		})
	}
}
