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

package policies

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

type fakeClient struct {
	teamMembers sets.String
}

func (f *fakeClient) ListTeamMembers(id int, role string) ([]github.TeamMember, error) {
	if id != 42 || role != github.RoleAll {
		return nil, errors.New("uh oh")
	}
	var res []github.TeamMember
	for _, member := range f.teamMembers.List() {
		res = append(res, github.TeamMember{Login: member})
	}
	return res, nil
}

func TestTeamAccess(t *testing.T) {
	var policy teamAccess
	policy = func(cfg *plugins.Configuration) (name string, id int) {
		return cfg.RepoMilestone[""].MaintainersTeam, cfg.RepoMilestone[""].MaintainersID
	}
	cfg := &plugins.Configuration{}
	cfg.RepoMilestone = map[string]plugins.Milestone{"": {MaintainersTeam: "sig-testing", MaintainersID: 42}}
	client := &fakeClient{teamMembers: sets.NewString("cjwagner", "BenTheElder", "stevekuznetsov")}

	for _, login := range []string{"cjwagner", "BENTHEELDER", "stevekuznetsov", "bentheelder"} {
		if allowed, err := policy.doCanAccess(cfg, client, login); err != nil {
			t.Errorf("Unexpected error: %v.", err)
		} else if !allowed {
			t.Errorf("Expected %q to be allowed access.", login)
		}
	}
	for _, login := range []string{"rando", "foo"} {
		if allowed, err := policy.doCanAccess(cfg, client, login); err != nil {
			t.Errorf("Unexpected error: %v.", err)
		} else if allowed {
			t.Errorf("Expected %q to be disallowed access.", login)
		}
	}
}
