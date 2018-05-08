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

package main

import (
	"testing"

	branchprotection "k8s.io/test-infra/prow/config"
)

func TestMakeRequest(t *testing.T) {
	actual := makeRequest(branchprotection.Policy{})
	if actual.RequiredStatusChecks.Contexts == nil {
		t.Errorf("contexts must be non-nil (use an empty list for nil)")
	}
	if actual.Restrictions.Teams == nil {
		t.Errorf("users must be non-nil (use an empty list for nil)")
	}
	if actual.Restrictions.Users == nil {
		t.Errorf("users must be non-nil (use an empty list for nil)")
	}
}
