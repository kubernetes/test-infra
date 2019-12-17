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
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"

	coreapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/plugins"
)

var (
	defaultNamespace = "default"
)

func TestRun(t *testing.T) {
	lg, c, err := localgit.New()
	if err != nil {
		t.Fatalf("Making local git repo: %v", err)
	}

	defer func() {
		if err := c.Clean(); err != nil {
			t.Errorf("Could not clean up git client cache: %v.", err)
		}
	}()

	if err := lg.MakeFakeRepo("openshift", "release"); err != nil {
		t.Fatalf("Making fake repo: %v", err)
	}
	if err := lg.Checkout("openshift", "release", "master"); err != nil {
		t.Fatalf("Checkout new branch: %v", err)
	}

	if err := lg.AddCommit("openshift", "release", map[string][]byte{
		"config/foo.yaml": []byte(`#foo.yaml`),
		"config/bar.yaml": []byte(`#bar.yaml`),
	}); err != nil {
		t.Fatalf("Add commit: %v", err)
	}

	testcases := []struct {
		name                      string
		sourcePath                string
		defaultNamespace          string
		configUpdater             plugins.ConfigUpdater
		buildClusterCoreV1Clients map[string]corev1.CoreV1Interface
		expected                  int

		existConfigMaps    []runtime.Object
		expectedConfigMaps []*coreapi.ConfigMap
	}{
		{
			name:             "issues/15570 is covered",
			sourcePath:       filepath.Join(lg.Dir, "openshift/release"),
			defaultNamespace: defaultNamespace,
			configUpdater: plugins.ConfigUpdater{
				Maps: map[string]plugins.ConfigMapSpec{
					"config/foo.yaml": {
						Name: "multikey-config",
						Clusters: map[string][]string{
							"default": {defaultNamespace},
						},
					},
					"config/bar.yaml": {
						Name: "multikey-config",
					},
				},
			},
			existConfigMaps: []runtime.Object{
				&coreapi.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multikey-config",
						Namespace: defaultNamespace,
					},
					Data: map[string]string{},
				},
			},
			expectedConfigMaps: []*coreapi.ConfigMap{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multikey-config",
						Namespace: defaultNamespace,
					},
					Data: map[string]string{
						"foo.yaml": "#foo.yaml",
						"bar.yaml": "#bar.yaml",
					},
				},
			},
		},
	}

	for _, tc := range testcases {
		fkc := fake.NewSimpleClientset(tc.existConfigMaps...)
		tc.configUpdater.SetDefaults()
		actual := run(tc.sourcePath, tc.defaultNamespace, tc.configUpdater, fkc, nil)
		if tc.expected != actual {
			t.Errorf("%s: incorrect errors '%d': expecting '%d'", tc.name, actual, tc.expected)
		}

		for _, expected := range tc.expectedConfigMaps {
			actual, err := fkc.CoreV1().ConfigMaps(expected.Namespace).Get(expected.Name, metav1.GetOptions{})
			if err != nil && errors.IsNotFound(err) {
				t.Errorf("%s: Should have updated or created configmap for '%s'", tc.name, expected)
			} else if !equality.Semantic.DeepEqual(expected, actual) {
				t.Errorf("%s: incorrect ConfigMap state after update: %v", tc.name, cmp.Diff(expected, actual))
			}
		}
	}

}
