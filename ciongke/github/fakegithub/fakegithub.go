/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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

package fakegithub

import (
	"github.com/kubernetes/test-infra/ciongke/github"
)

type OrgMember struct {
	Org  string
	User string
}

type TeamMember struct {
	Team int
	User string
}

type FakeClient struct {
	OrgMembers  []OrgMember
	TeamMembers []TeamMember
}

func (f *FakeClient) IsMember(org, user string) (bool, error) {
	for _, m := range f.OrgMembers {
		if m.Org == org && m.User == user {
			return true, nil
		}
	}
	return false, nil
}

func (f *FakeClient) IsTeamMember(team int, user string) (bool, error) {
	for _, m := range f.TeamMembers {
		if m.Team == team && m.User == user {
			return true, nil
		}
	}
	return false, nil
}

func (f *FakeClient) CreateStatus(owner, repo, ref string, s github.Status) error {
	return nil
}
