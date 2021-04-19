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

package main

import (
	"flag"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/cmd/hmac/fakeghhook"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/github"
)

func TestGatherOptions(t *testing.T) {
	cases := []struct {
		name     string
		args     map[string]string
		del      sets.String
		expected func(*options)
		err      bool
	}{
		{
			name: "minimal flags work",
		},
		{
			name: "explicitly set --config-path",
			args: map[string]string{
				"--config-path": "/random/value",
			},
			expected: func(o *options) {
				o.config.ConfigPath = "/random/value"
			},
		},
		{
			name: "explicitly set --dry-run=false",
			args: map[string]string{
				"--dry-run": "false",
			},
			expected: func(o *options) {
				o.dryRun = false
			},
		},
		{
			name: "--dry-run=true requires --deck-url",
			args: map[string]string{
				"--dry-run":  "true",
				"--deck-url": "",
			},
			err: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ghoptions := flagutil.GitHubOptions{}
			ghoptions.AddFlags(&flag.FlagSet{})
			ghoptions.Validate(false)
			expected := &options{
				config: configflagutil.ConfigOptions{
					ConfigPathFlagName:              "config-path",
					JobConfigPathFlagName:           "job-config-path",
					ConfigPath:                      "yo",
					SupplementalProwConfigsFileName: "_prowconfig.yaml",
				},
				dryRun:                   true,
				github:                   ghoptions,
				kubernetes:               flagutil.KubernetesOptions{DeckURI: "http://whatever-deck-url"},
				kubeconfigCtx:            "whatever-kubeconfig-context",
				hookUrl:                  "http://whatever-hook-url",
				hmacTokenSecretNamespace: "default",
				hmacTokenSecretName:      "hmac-token",
				hmacTokenKey:             "hmac",
			}
			if tc.expected != nil {
				tc.expected(expected)
			}

			argMap := map[string]string{
				"--config-path":            "yo",
				"--deck-url":               "http://whatever-deck-url",
				"--hook-url":               "http://whatever-hook-url",
				"--kubeconfig-context":     "whatever-kubeconfig-context",
				"--hmac-token-secret-name": "hmac-token",
				"--hmac-token-key":         "hmac",
			}
			for k, v := range tc.args {
				argMap[k] = v
			}
			for k := range tc.del {
				delete(argMap, k)
			}

			var args []string
			for k, v := range argMap {
				args = append(args, k+"="+v)
			}
			fs := flag.NewFlagSet("fake-flags", flag.PanicOnError)
			actual := gatherOptions(fs, args...)
			switch err := actual.validate(); {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Errorf("failed to receive expected error")
			case !reflect.DeepEqual(*expected, actual):
				t.Errorf("\n%#v\n != expected \n%#v\n", actual, *expected)
			}
		})
	}
}

func TestPruneOldTokens(t *testing.T) {
	// "2006-01-02T15:04:05+07:00"
	time1, _ := time.Parse(time.RFC3339, "2020-01-05T19:07:08+00:00")
	time2, _ := time.Parse(time.RFC3339, "2020-02-05T19:07:08+00:00")
	time3, _ := time.Parse(time.RFC3339, "2020-03-05T19:07:08+00:00")

	cases := []struct {
		name     string
		current  map[string]github.HMACsForRepo
		repo     string
		expected map[string]github.HMACsForRepo
	}{
		{
			name: "three hmacs, only the latest one is left after pruning",
			current: map[string]github.HMACsForRepo{
				"org1/repo1": []github.HMACToken{
					{
						Value:     "rand-val1",
						CreatedAt: time1,
					},
					{
						Value:     "rand-val2",
						CreatedAt: time2,
					},
					{
						Value:     "rand-val3",
						CreatedAt: time3,
					},
				},
			},
			repo: "org1/repo1",
			expected: map[string]github.HMACsForRepo{
				"org1/repo1": []github.HMACToken{
					{
						Value:     "rand-val3",
						CreatedAt: time3,
					},
				},
			},
		},
		{
			name: "two hmacs, only the latest one is left after pruning",
			current: map[string]github.HMACsForRepo{
				"org1/repo1": []github.HMACToken{
					{
						Value:     "rand-val1",
						CreatedAt: time1,
					},
					{
						Value:     "rand-val2",
						CreatedAt: time2,
					},
				},
			},
			repo: "org1/repo1",
			expected: map[string]github.HMACsForRepo{
				"org1/repo1": []github.HMACToken{
					{
						Value:     "rand-val2",
						CreatedAt: time2,
					},
				},
			},
		},
		{
			name: "nothing will be changed if the repo is not in the map",
			current: map[string]github.HMACsForRepo{
				"org1/repo1": []github.HMACToken{
					{
						Value:     "rand-val1",
						CreatedAt: time1,
					},
				},
			},
			repo: "org2/repo2",
			expected: map[string]github.HMACsForRepo{
				"org1/repo1": []github.HMACToken{
					{
						Value:     "rand-val1",
						CreatedAt: time1,
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &client{currentHMACMap: tc.current}
			c.pruneOldTokens(tc.repo)
			if !reflect.DeepEqual(tc.expected, c.currentHMACMap) {
				t.Errorf("%#v != expected %#v", c.currentHMACMap, tc.expected)
			}
		})
	}
}

func TestGenerateNewHMACToken(t *testing.T) {
	token1, err := generateNewHMACToken()
	if err != nil {
		t.Errorf("error generating new hmac token1: %v", err)
	}

	token2, err := generateNewHMACToken()
	if err != nil {
		t.Errorf("error generating new hmac token2: %v", err)
	}
	if token1 == token2 {
		t.Error("the generated hmac token should be random, but the two are equal")
	}
}

func TestHandleRemovedRepo(t *testing.T) {
	cases := []struct {
		name          string
		toRemove      map[string]bool
		expectedHMACs map[string]github.HMACsForRepo
		expectedHooks map[string][]github.Hook
	}{
		{
			name:     "delete hmac and hook for one repo",
			toRemove: map[string]bool{"repo1": true},
			expectedHMACs: map[string]github.HMACsForRepo{
				"repo2": {
					github.HMACToken{
						Value: "val2",
					},
				},
			},
			expectedHooks: map[string][]github.Hook{
				"repo2": {
					github.Hook{
						ID:     0,
						Name:   "hook2",
						Active: true,
						Config: github.HookConfig{
							URL: "http://whatever-hook-url",
						},
					},
				},
			},
		},
		{
			name:          "delete hmac and hook for multiple repos",
			toRemove:      map[string]bool{"repo1": true, "repo2": true},
			expectedHMACs: map[string]github.HMACsForRepo{},
			expectedHooks: map[string][]github.Hook{},
		},
		{
			name:     "delete hmac and hook for non-existed repo",
			toRemove: map[string]bool{"repo1": true, "whatever-repo": true},
			expectedHMACs: map[string]github.HMACsForRepo{
				"repo2": {
					github.HMACToken{
						Value: "val2",
					},
				},
			},
			expectedHooks: map[string][]github.Hook{
				"repo2": {
					github.Hook{
						Name:   "hook2",
						Active: true,
						Config: github.HookConfig{
							URL: "http://whatever-hook-url",
						},
					},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClient := &fakeghhook.FakeClient{
				OrgHooks: map[string][]github.Hook{
					"repo1": {
						github.Hook{
							ID:     0,
							Name:   "hook1",
							Active: true,
							Config: github.HookConfig{
								URL: "http://whatever-hook-url",
							},
						},
					},
					"repo2": {
						github.Hook{
							ID:     0,
							Name:   "hook2",
							Active: true,
							Config: github.HookConfig{
								URL: "http://whatever-hook-url",
							},
						},
					},
				},
			}
			c := &client{
				currentHMACMap: map[string]github.HMACsForRepo{
					"repo1": {
						github.HMACToken{
							Value: "val1",
						},
					},
					"repo2": {
						github.HMACToken{
							Value: "val2",
						},
					},
				},
				githubHookClient: fakeClient,
				options:          options{hookUrl: "http://whatever-hook-url"},
			}
			if err := c.handleRemovedRepo(tc.toRemove); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(tc.expectedHMACs, c.currentHMACMap) {
				t.Errorf("hmacs %#v != expected %#v", c.currentHMACMap, tc.expectedHMACs)
			}
			if !reflect.DeepEqual(tc.expectedHooks, fakeClient.OrgHooks) {
				t.Errorf("hooks %#v != expected %#v", fakeClient.OrgHooks, tc.expectedHooks)
			}
		})
	}
}

func TestHandleAddedRepo(t *testing.T) {
	globalToken := []github.HMACToken{
		{
			Value:     "global-rand-val1",
			CreatedAt: time.Now().Add(-time.Hour),
		},
	}

	cases := []struct {
		name                         string
		toAdd                        map[string]config.ManagedWebhookInfo
		currentHMACs                 map[string]github.HMACsForRepo
		currentHMACMapForBatchUpdate map[string]string
		expectedHMACsSize            map[string]int
	}{
		{
			name: "add repos when global token does not exist",
			toAdd: map[string]config.ManagedWebhookInfo{
				"repo1": {TokenCreatedAfter: time.Now()},
				"repo2": {TokenCreatedAfter: time.Now()},
			},
			currentHMACs:                 map[string]github.HMACsForRepo{},
			currentHMACMapForBatchUpdate: map[string]string{"whatever-repo": "whatever-token"},
			expectedHMACsSize:            map[string]int{"repo1": 1, "repo2": 1},
		},
		{
			name: "add repos when global token exists",
			toAdd: map[string]config.ManagedWebhookInfo{
				"repo1": {TokenCreatedAfter: time.Now()},
				"repo2": {TokenCreatedAfter: time.Now()},
			},
			currentHMACs: map[string]github.HMACsForRepo{
				"*": globalToken,
			},
			currentHMACMapForBatchUpdate: map[string]string{"whatever-repo": "whatever-token"},
			expectedHMACsSize:            map[string]int{"repo1": 2, "repo2": 2},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &client{
				currentHMACMap:        tc.currentHMACs,
				hmacMapForBatchUpdate: tc.currentHMACMapForBatchUpdate,
			}
			if err := c.handleAddedRepo(tc.toAdd); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			for repo, size := range tc.expectedHMACsSize {
				if _, ok := c.currentHMACMap[repo]; !ok {
					t.Errorf("repo %q does not exist in the updated HMAC map", repo)
				} else if len(c.currentHMACMap[repo]) != size {
					t.Errorf("repo %q hmac size %d != expected %d", repo, len(c.currentHMACMap[repo]), size)
				}
			}
			for repo := range tc.toAdd {
				if _, ok := c.hmacMapForBatchUpdate[repo]; !ok {
					t.Errorf("repo %q is expected to be added to the batch update map, but not", repo)
				}
			}
		})
	}
}

func TestHandleRotatedRepo(t *testing.T) {
	pastTime, _ := time.Parse(time.RFC3339Nano, "2020-01-01T00:00:50Z")

	globalToken := []github.HMACToken{
		{
			Value:     "global-rand-val1",
			CreatedAt: pastTime,
		},
	}
	commonTokens := []github.HMACToken{
		{
			Value:     "rand-val1",
			CreatedAt: pastTime,
		},
		{
			Value:     "rand-val2",
			CreatedAt: pastTime,
		},
	}

	cases := []struct {
		name                         string
		toRotate                     map[string]config.ManagedWebhookInfo
		currentHMACs                 map[string]github.HMACsForRepo
		currentHMACMapForBatchUpdate map[string]string
		expectedHMACsSize            map[string]int
		expectedReposForBatchUpdate  []string
		expectedHMACMapForRecovery   map[string]github.HMACsForRepo
	}{
		{
			name: "test a repo that needs its hmac to be rotated, and global token does not exist",
			toRotate: map[string]config.ManagedWebhookInfo{
				"repo1": {TokenCreatedAfter: time.Now()},
				"repo2": {TokenCreatedAfter: time.Now()},
			},
			currentHMACs: map[string]github.HMACsForRepo{
				"repo1": commonTokens,
				"repo2": commonTokens,
			},
			currentHMACMapForBatchUpdate: map[string]string{"whatever-repo": "whatever-token"},
			expectedHMACsSize:            map[string]int{"repo1": 3, "repo2": 3},
			expectedReposForBatchUpdate:  []string{"repo1", "repo2"},
			expectedHMACMapForRecovery: map[string]github.HMACsForRepo{
				"repo1": commonTokens,
				"repo2": commonTokens,
			},
		},
		{
			name: "test a repo that needs its hmac to be rotated, and global token exists",
			toRotate: map[string]config.ManagedWebhookInfo{
				"repo1": {TokenCreatedAfter: time.Now()},
				"repo2": {TokenCreatedAfter: time.Now()},
			},
			currentHMACs: map[string]github.HMACsForRepo{
				"*":     globalToken,
				"repo1": commonTokens,
				"repo2": commonTokens,
			},
			currentHMACMapForBatchUpdate: map[string]string{"whatever-repo": "whatever-token"},
			expectedHMACsSize:            map[string]int{"repo1": 3, "repo2": 3},
			expectedReposForBatchUpdate:  []string{"repo1", "repo2"},
			expectedHMACMapForRecovery: map[string]github.HMACsForRepo{
				"repo1": commonTokens,
				"repo2": commonTokens,
			},
		},
		{
			name: "test a repo that does not need its hmac to be rotated",
			toRotate: map[string]config.ManagedWebhookInfo{
				"repo1": {TokenCreatedAfter: pastTime},
				"repo2": {TokenCreatedAfter: pastTime},
			},
			currentHMACs: map[string]github.HMACsForRepo{
				"repo1": []github.HMACToken{
					{
						Value:     "rand-val1",
						CreatedAt: pastTime.Add(-1 * time.Hour),
					},
				},
				"repo2": []github.HMACToken{
					{
						Value:     "rand-val2",
						CreatedAt: pastTime.Add(1 * time.Hour),
					},
				},
			},
			currentHMACMapForBatchUpdate: map[string]string{"whatever-repo": "whatever-token"},
			expectedHMACsSize:            map[string]int{"repo1": 2, "repo2": 1},
			expectedReposForBatchUpdate:  []string{"repo1"},
			expectedHMACMapForRecovery: map[string]github.HMACsForRepo{
				"repo1": []github.HMACToken{
					{
						Value:     "rand-val1",
						CreatedAt: pastTime.Add(-1 * time.Hour),
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &client{
				currentHMACMap:        tc.currentHMACs,
				hmacMapForBatchUpdate: tc.currentHMACMapForBatchUpdate,
				hmacMapForRecovery:    map[string]github.HMACsForRepo{},
			}
			if err := c.handledRotatedRepo(tc.toRotate); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			for repo, size := range tc.expectedHMACsSize {
				if _, ok := c.currentHMACMap[repo]; !ok {
					t.Errorf("repo %q does not exist in the updated HMAC map", repo)
				} else if len(c.currentHMACMap[repo]) != size {
					t.Errorf("repo %q hmac size %d != expected %d", repo, len(c.currentHMACMap[repo]), size)
				}
			}
			for _, repo := range tc.expectedReposForBatchUpdate {
				if _, ok := c.hmacMapForBatchUpdate[repo]; !ok {
					t.Errorf("repo %q is expected to be added to the batch update map, but not", repo)
				}
			}
			if !reflect.DeepEqual(tc.expectedHMACMapForRecovery, c.hmacMapForRecovery) {
				t.Errorf("The hmacMapForRecovery %#v != expected %#v", c.hmacMapForRecovery, tc.expectedHMACMapForRecovery)
			}
		})
	}
}

func TestBatchOnboardNewTokenForRepos(t *testing.T) {
	name := "web"
	contentType := "json"
	secretBeforeUpdate := "whatever-secret-before-update"
	secretAfterUpdate := "whatever-secret-after-update"
	hookBeforeUpdate := github.Hook{
		ID:     0,
		Name:   name,
		Active: true,
		Events: github.AllHookEvents,
		Config: github.HookConfig{
			URL:         "http://whatever-hook-url",
			ContentType: &contentType,
			Secret:      &secretBeforeUpdate,
		},
	}
	hookAfterUpdate := github.Hook{
		ID:     0,
		Name:   name,
		Active: true,
		Events: github.AllHookEvents,
		Config: github.HookConfig{
			URL:         "http://whatever-hook-url",
			ContentType: &contentType,
			Secret:      &secretAfterUpdate,
		},
	}

	cases := []struct {
		name                  string
		hmacMapForBatchUpdate map[string]string
		currentOrgHooks       map[string][]github.Hook
		currentRepoHooks      map[string][]github.Hook
		expectedOrgHooks      map[string][]github.Hook
		expectedRepoHooks     map[string][]github.Hook
	}{
		{
			name:                  "add hook for one repo",
			hmacMapForBatchUpdate: map[string]string{"org/repo1": secretBeforeUpdate},
			currentRepoHooks:      map[string][]github.Hook{},
			expectedRepoHooks: map[string][]github.Hook{
				"org/repo1": {hookBeforeUpdate},
			},
		},
		{
			name:                  "add hook for one org",
			hmacMapForBatchUpdate: map[string]string{"org1": secretBeforeUpdate},
			currentOrgHooks:       map[string][]github.Hook{},
			expectedOrgHooks: map[string][]github.Hook{
				"org1": {hookBeforeUpdate},
			},
		},
		{
			name:                  "update hook for one org",
			hmacMapForBatchUpdate: map[string]string{"org1": secretAfterUpdate},
			currentOrgHooks: map[string][]github.Hook{
				"org1": {hookBeforeUpdate},
			},
			expectedOrgHooks: map[string][]github.Hook{
				"org1": {hookAfterUpdate},
			},
		},
		{
			name:                  "update hook for one repo",
			hmacMapForBatchUpdate: map[string]string{"org/repo1": secretAfterUpdate},
			currentRepoHooks: map[string][]github.Hook{
				"org/repo1": {hookBeforeUpdate},
			},
			expectedRepoHooks: map[string][]github.Hook{
				"org/repo1": {hookAfterUpdate},
			},
		},
		{
			name:                  "add hook for one org, and update hook for one repo",
			hmacMapForBatchUpdate: map[string]string{"org1": secretAfterUpdate, "org2/repo": secretAfterUpdate},
			currentOrgHooks:       map[string][]github.Hook{},
			expectedOrgHooks: map[string][]github.Hook{
				"org1": {hookAfterUpdate},
			},
			currentRepoHooks: map[string][]github.Hook{
				"org2/repo": {hookBeforeUpdate},
			},
			expectedRepoHooks: map[string][]github.Hook{
				"org2/repo": {hookAfterUpdate},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fakeclient := &fakeghhook.FakeClient{
				OrgHooks:  tc.currentOrgHooks,
				RepoHooks: tc.currentRepoHooks,
			}
			c := &client{
				githubHookClient:      fakeclient,
				hmacMapForBatchUpdate: tc.hmacMapForBatchUpdate,
				options:               options{hookUrl: "http://whatever-hook-url"},
			}
			if err := c.batchOnboardNewTokenForRepos(); err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(fakeclient.OrgHooks, tc.expectedOrgHooks) {
				t.Errorf("org hooks %#v != expected %#v", fakeclient.OrgHooks, tc.expectedOrgHooks)
			}
			if !reflect.DeepEqual(fakeclient.RepoHooks, tc.expectedRepoHooks) {
				t.Errorf("repo hooks %#v != expected %#v", fakeclient.RepoHooks, tc.expectedRepoHooks)
			}
		})
	}
}

func TestHandleInvitation(t *testing.T) {
	tests := []struct {
		name          string
		urivs         []github.UserRepoInvitation
		uoivs         []github.UserOrgInvitation
		newHMACConfig config.ManagedWebhooks
		wantUrivs     []github.UserRepoInvitation
		wantUoivs     []github.UserOrgInvitation
		wantErr       error
	}{
		{
			name: "accept repo invitation",
			urivs: []github.UserRepoInvitation{
				{
					Repository: &github.Repo{
						FullName: "org1/repo1",
					},
					Permission: "admin",
				},
			},
			newHMACConfig: config.ManagedWebhooks{
				AutoAcceptInvitation: true,
				OrgRepoConfig: map[string]config.ManagedWebhookInfo{
					"org1/repo1": {},
				},
			},
			wantUrivs: []github.UserRepoInvitation{},
		},
		{
			name: "accept org invitation",
			uoivs: []github.UserOrgInvitation{
				{
					Org: github.UserOrganization{
						Login: "org1",
					},
					Role: "admin",
				},
			},
			newHMACConfig: config.ManagedWebhooks{
				AutoAcceptInvitation: true,
				OrgRepoConfig: map[string]config.ManagedWebhookInfo{
					"org1": {},
				},
			},
			wantUoivs: []github.UserOrgInvitation{},
		},
		{
			name: "accept org invitation with single repo webhook",
			uoivs: []github.UserOrgInvitation{
				{
					Org: github.UserOrganization{
						Login: "org1",
					},
					Role: "admin",
				},
			},
			newHMACConfig: config.ManagedWebhooks{
				AutoAcceptInvitation: true,
				OrgRepoConfig: map[string]config.ManagedWebhookInfo{
					"org1/repo1": {},
				},
			},
			wantUoivs: []github.UserOrgInvitation{},
		},
		{
			name: "dont accept repo invitation with org webhook",
			urivs: []github.UserRepoInvitation{
				{
					Repository: &github.Repo{
						FullName: "org1/repo1",
					},
					Permission: "admin",
				},
			},
			newHMACConfig: config.ManagedWebhooks{
				AutoAcceptInvitation: true,
				OrgRepoConfig: map[string]config.ManagedWebhookInfo{
					"org1": {},
				},
			},
			wantUrivs: []github.UserRepoInvitation{
				{
					Repository: &github.Repo{
						FullName: "org1/repo1",
					},
					Permission: "admin",
				},
			},
		},
		{
			name: "dont accept invitation when opt out",
			urivs: []github.UserRepoInvitation{
				{
					Repository: &github.Repo{
						FullName: "org1/repo1",
					},
					Permission: "admin",
				},
			},
			uoivs: []github.UserOrgInvitation{
				{
					Org: github.UserOrganization{
						Login: "org2",
					},
					Role: "admin",
				},
			},
			newHMACConfig: config.ManagedWebhooks{
				AutoAcceptInvitation: false,
				OrgRepoConfig: map[string]config.ManagedWebhookInfo{
					"org2":       {},
					"org1/repo1": {},
				},
			},
			wantUrivs: []github.UserRepoInvitation{
				{
					Repository: &github.Repo{
						FullName: "org1/repo1",
					},
					Permission: "admin",
				},
			},
			wantUoivs: []github.UserOrgInvitation{
				{
					Org: github.UserOrganization{
						Login: "org2",
					},
					Role: "admin",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fgc := fakeghhook.FakeClient{
				UserRepoInvitations: tc.urivs,
				UserOrgInvitations:  tc.uoivs,
			}
			c := client{
				newHMACConfig:    tc.newHMACConfig,
				githubHookClient: &fgc,
			}

			if wantErr, gotErr := tc.wantErr, c.handleInvitation(); (wantErr == nil && gotErr != nil) || (wantErr != nil && gotErr == nil) ||
				(wantErr != nil && gotErr != nil && !strings.Contains(gotErr.Error(), wantErr.Error())) {
				t.Fatalf("Error mismatch. Want: %v, got: %v", wantErr, gotErr)
			}
			if diff := cmp.Diff(tc.wantUrivs, fgc.UserRepoInvitations); diff != "" {
				t.Fatalf("User repo invitation mismatch. Want(-), got(+): %s", diff)
			}
			if diff := cmp.Diff(tc.wantUoivs, fgc.UserOrgInvitations); diff != "" {
				t.Fatalf("User org invitation mismatch. Want(-), got(+): %s", diff)
			}
		})
	}
}
