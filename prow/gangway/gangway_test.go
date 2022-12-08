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

package gangway

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGetOrgRepo(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want []string
	}{
		{
			name: "GitHub url",
			url:  "https://github.com/kubernetes/test-infra",
			want: []string{"kubernetes", "test-infra"},
		},
		{
			name: "Gerrit url",
			url:  "https://linux-review.googlesource.com/linux/kernel/git/torvalds/linux",
			want: []string{"https://linux-review.googlesource.com/linux/kernel/git/torvalds", "linux"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			org, repo, _ := getOrgRepo(tc.url)
			if diff := cmp.Diff(tc.want, []string{org, repo}); diff != "" {
				t.Errorf("orgRepo mismatch. Want(-), got(+):\n%s", diff)
			}
		})
	}
}
