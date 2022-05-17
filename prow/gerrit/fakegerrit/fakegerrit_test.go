/*
Copyright 2022 The Kubernetes Authors.

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

package fakegerrit

import (
	"testing"

	gerrit "github.com/andygrunwald/go-gerrit"
)

func TestAddChange(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "added properlly for new project",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fg := NewFakeGerritClient()
			change := gerrit.ChangeInfo{ChangeID: "1"}
			fg.AddChange("testproject", &change)

			if project, ok := fg.Projects["testproject"]; !ok {
				t.Fatalf("project %s not properly added to fake gerrit client", "testproject")
			} else {
				if len(project.ChangeIDs) == 0 {
					t.Fatalf("change not properly added to project")
				}
			}
		})
	}
}
