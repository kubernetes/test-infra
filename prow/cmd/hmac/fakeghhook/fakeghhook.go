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

package fakeghhook

import (
	"fmt"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

// FakeClient is like client, but fake.
type FakeClient struct {
	// Maps org name to the list of hooks
	OrgHooks map[string][]github.Hook
	// Maps repo name to the list of hooks
	RepoHooks map[string][]github.Hook
}

func (f *FakeClient) ListOrgHooks(org string) ([]github.Hook, error) {
	return listHooks(f.OrgHooks, org)
}

func (f *FakeClient) ListRepoHooks(org, repo string) ([]github.Hook, error) {
	logrus.Infof("list hooks for %q", org+"/"+repo)
	return listHooks(f.RepoHooks, org+"/"+repo)
}

func listHooks(m map[string][]github.Hook, key string) ([]github.Hook, error) {
	return m[key], nil
}

func (f *FakeClient) CreateOrgHook(org string, req github.HookRequest) (int, error) {
	if err := createHook(f.OrgHooks, org, req); err != nil {
		return -1, err
	}
	return 0, nil
}

func (f *FakeClient) CreateRepoHook(org, repo string, req github.HookRequest) (int, error) {
	if err := createHook(f.RepoHooks, org+"/"+repo, req); err != nil {
		return -1, err
	}
	return 0, nil
}

func createHook(m map[string][]github.Hook, key string, req github.HookRequest) error {
	var hooks []github.Hook
	if _, ok := m[key]; ok {
		hooks = m[key]
	}

	for _, hook := range hooks {
		if hook.Config.URL == req.Config.URL {
			return fmt.Errorf("error creating the hook as %q already exists", hook.Config.URL)
		}
	}
	m[key] = append(hooks, github.Hook{
		ID:     0,
		Name:   "web",
		Events: req.Events,
		Active: *req.Active,
		Config: *req.Config,
	})
	return nil
}

func (f *FakeClient) EditOrgHook(org string, id int, req github.HookRequest) error {
	return editHook(f.OrgHooks, org, req)
}

func (f *FakeClient) EditRepoHook(org, repo string, id int, req github.HookRequest) error {
	return editHook(f.RepoHooks, org+"/"+repo, req)
}

func editHook(m map[string][]github.Hook, key string, req github.HookRequest) error {
	if _, ok := m[key]; !ok {
		return fmt.Errorf("no hooks exist for %q, cannot edit", key)
	}

	exists := false
	hooks := m[key]
	for i, hook := range hooks {
		if hook.Config.URL == req.Config.URL {
			hooks[i] = github.Hook{
				Name:   "web",
				Events: req.Events,
				Active: *req.Active,
				Config: *req.Config,
			}
			exists = true
		}
	}

	if !exists {
		return fmt.Errorf("no hook for %q, cannot edit", req.Config.URL)
	}

	m[key] = hooks
	return nil
}

func (f *FakeClient) DeleteOrgHook(org string, id int, req github.HookRequest) error {
	return deleteHook(f.OrgHooks, org, req)
}

func (f *FakeClient) DeleteRepoHook(org, repo string, id int, req github.HookRequest) error {
	return deleteHook(f.RepoHooks, org+"/"+repo, req)
}

func deleteHook(m map[string][]github.Hook, key string, req github.HookRequest) error {
	if _, ok := m[key]; !ok {
		return nil
	}

	hookExists := false
	for i, hook := range m[key] {
		if hook.Config.URL == req.Config.URL {
			hookExists = true
			m[key] = append(m[key][:i], m[key][i+1:]...)
			if len(m[key]) == 0 {
				delete(m, key)
			}
			return nil
		}
	}

	if !hookExists {
		return fmt.Errorf("hook for %q does not exist, cannot delete", req.Config.URL)
	}
	return nil
}
