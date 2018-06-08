/*
Copyright 2018 The Kubernetes Authors.

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

package spyglass

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"
)

func TestGCSFetchArtifacts(t *testing.T) {
	t.Logf("Begin")
	fmt.Printf("Begin2")
	server := fakestorage.NewServer([]fakestorage.Object{
		{
			BucketName: "test-bucket",
			Name:       "logs/example-ci-run/403/build-log.txt",
			Content:    []byte("Oh wow\nlogs\nthis is\ncrazy\n"),
		},
		{
			BucketName: "test-bucket",
			Name:       "logs/example-ci-run/403/started.json",
			Content: []byte(`{
						  "node": "gke-prow-default-pool-3c8994a8-qfhg", 
						  "repo-version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "timestamp": 1528742858, 
						  "repos": {
						    "k8s.io/kubernetes": "master", 
						    "k8s.io/release": "master"
						  }, 
						  "version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "metadata": {
						    "pod": "cbc53d8e-6da7-11e8-a4ff-0a580a6c0269"
						  }
						}`),
		},
		{
			BucketName: "test-bucket",
			Name:       "logs/example-ci-run/403/finished.json",
			Content: []byte(`{
						  "timestamp": 1528742943, 
						  "version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "result": "SUCCESS", 
						  "passed": true, 
						  "job-version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "metadata": {
						    "repo": "k8s.io/kubernetes", 
						    "repos": {
						      "k8s.io/kubernetes": "master", 
						      "k8s.io/release": "master"
						    }, 
						    "infra-commit": "260081852", 
						    "pod": "cbc53d8e-6da7-11e8-a4ff-0a580a6c0269", 
						    "repo-commit": "e6f64d0a79243c834babda494151fc5d66582240"
						  },
						},`),
		},
	})
	defer server.Stop()
	fmt.Printf("%s", server.URL())
	fakeGCSClient := server.Client()
	testAf := &GCSArtifactFetcher{
		client: fakeGCSClient,
	}
	testCases := []struct {
		name              string
		gcsJobSource      GCSJobSource
		expectedArtifacts []Artifact
	}{
		{
			name: "Fetch Example CI Run #403 Artifacts",
			gcsJobSource: GCSJobSource{
				bucket:  "test-bucket",
				jobPath: "logs/example-ci-run/403",
			},
			expectedArtifacts: []Artifact{
				GCSArtifact{
					link: "https://localhost:8080/test-bucket/logs/example-ci-run/403/build-log.txt",
					path: "build-log.txt",
				},
				GCSArtifact{
					link: "https://localhost:8080/test-bucket/logs/example-ci-run/403/started.json",
					path: "started.json",
				},
				GCSArtifact{
					link: "https://localhost:8080/test-bucket/logs/example-ci-run/403/finished.json",
					path: "finished.json",
				},
			},
		},
	}

	for _, tc := range testCases {
		actualArtifacts := testAf.Artifacts(&tc.gcsJobSource)
		t.Errorf("%s", actualArtifacts)
		fmt.Printf("%s", tc.expectedArtifacts)
		for _, ea := range tc.expectedArtifacts {
			found := false
			for _, aa := range actualArtifacts {
				if reflect.DeepEqual(ea, aa) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Case %s failed to retrieve the following artifact: %s\nRetrieved: %s.", tc.name, ea, actualArtifacts)
			}

		}
		if len(tc.expectedArtifacts) != len(actualArtifacts) {
			t.Errorf("Case %s produced more artifacts than expected. Expected: %s\nActual: %s.", tc.name, tc.expectedArtifacts, actualArtifacts)
		}
	}
}
