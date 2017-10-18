/*
Copyright 2017 The Kubernetes Authors.

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

package github

import (
	"testing"
)

func TestIssueCapsLogin(t *testing.T) {
	// some valid logins that should all normalize to match the first
	validLoginVariants := []string{
		"BenTheElder",
		"BENTHEELDER",
		"bentheelder",
		"BenTHEElder",
		"BeNtHeElDeR",
		"bEnThEeLdEr",
	}
	// add an explicitly normalized version for sanity
	validLoginVariants = append(validLoginVariants, NormLogin(validLoginVariants[0]))

	issue := Issue{
		User: User{
			Login: validLoginVariants[0],
		},
		Assignees: []User{
			{
				Login: validLoginVariants[0],
			},
		},
	}
	for _, login := range validLoginVariants {
		if !issue.IsAuthor(login) {
			t.Errorf("expected issue.IsAuthor(%s) to be true", login)
		}
		if !issue.IsAssignee(login) {
			t.Errorf("expected issue.IsAssignee(%s) to be true", login)
		}
	}
}
