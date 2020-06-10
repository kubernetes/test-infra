/*
Copyright 2020 The Kubernetes Authors.

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

package deployer

import (
	"testing"
)

func TestSetEmptyRepoPath(t *testing.T) {
	d := &deployer{}

	err := d.setRepoPathIfNotSet()

	if err != nil {
		t.Fatal(err)
	}

	if len(d.repoRootPath) == 0 {
		t.Error("expected new root path to be non-empty")
	}
}

func TestSetPopulatedRepoPath(t *testing.T) {
	path := "/test/path"
	d := &deployer{
		repoRootPath: path,
	}

	err := d.setRepoPathIfNotSet()

	if err != nil {
		t.Fatal(err)
	}

	if d.repoRootPath != path {
		t.Error("repo root path after call is supposed to be the same as before the call")
	}
}
