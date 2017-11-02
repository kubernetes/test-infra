/*
Copyright 2015 The Kubernetes Authors.

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

package utils

import (
	"testing"
)

func TestExpandListURL(t *testing.T) {
	table := []struct {
		bucket    string
		pathParts []interface{}
		expect    string
	}{
		{
			bucket:    "kubernetes-jenkins",
			pathParts: []interface{}{"logs", "kubernetes-gce-e2e", 1458, "artifacts", "junit"},
			expect:    "https://www.googleapis.com/storage/v1/b/kubernetes-jenkins/o?prefix=logs%2Fkubernetes-gce-e2e%2F1458%2Fartifacts%2Fjunit",
		},
	}

	for _, tt := range table {
		b := NewBucket(tt.bucket)
		out := b.ExpandListURL(tt.pathParts...).String()
		if out != tt.expect {
			t.Errorf("Expected %v but got %v", tt.expect, out)
		}
	}
}
