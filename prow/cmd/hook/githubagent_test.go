/*
Copyright 2016 The Kubernetes Authors.

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

	"k8s.io/test-infra/prow/github/fakegithub"
)

// Make sure that we use the cache for members but ignore it for non-members.
func TestIsMemberCache(t *testing.T) {
	gh := &fakegithub.FakeClient{
		OrgMembers: []string{"t"},
	}
	ga := &GitHubAgent{
		GitHubClient: gh,
	}
	if member, err := ga.isMember("t"); err != nil {
		t.Errorf("Didn't expect error testing membership the first time: %v", err)
	} else if !member {
		t.Error("Expected user \"t\" to be a member.")
	}
	if !ga.orgMembers["t"] {
		t.Error("Didn't cache org membership.")
	}
	gh.OrgMembers = []string{}
	if member, err := ga.isMember("t"); err != nil {
		t.Errorf("Didn't expect error testing membership the second time: %v", err)
	} else if !member {
		t.Error("Expected user \"t\" to be a member from cache.")
	}

	if member, err := ga.isMember("u"); err != nil {
		t.Errorf("Didn't expect error testing membership the first time: %v", err)
	} else if member {
		t.Error("Expected user \"u\" to not be a member.")
	}
	gh.OrgMembers = []string{"u"}
	if member, err := ga.isMember("u"); err != nil {
		t.Errorf("Didn't expect error testing membership the second time: %v", err)
	} else if !member {
		t.Error("Expected user \"u\" to be a member.")
	}

}
