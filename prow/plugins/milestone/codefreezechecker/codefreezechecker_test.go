/*
Copyright 2023 The Kubernetes Authors.

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

package codefreezechecker

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/test-infra/prow/config"
)

func TestInCodeFreeze(t *testing.T) {
	const (
		testOrg           = "org"
		testRepo          = "repo"
		testMilestone     = "v1.29"
		testReleaseBranch = "release-1.29"
	)
	t.Parallel()
	for _, tc := range []struct {
		name            string
		config          *config.Config
		isInCodeFreezte bool
	}{
		{
			name: "in code freeze",
			config: &config.Config{
				ProwConfig: config.ProwConfig{
					Tide: config.Tide{
						TideGitHubConfig: config.TideGitHubConfig{
							Queries: config.TideQueries{
								{
									Milestone:        testMilestone,
									Repos:            []string{fmt.Sprintf("%s/%s", testOrg, testRepo)},
									IncludedBranches: []string{testReleaseBranch, defaultBranches[0]},
								},
							},
						},
					},
				},
			},
			isInCodeFreezte: true,
		},
		{
			name: "different milestone",
			config: &config.Config{
				ProwConfig: config.ProwConfig{
					Tide: config.Tide{
						TideGitHubConfig: config.TideGitHubConfig{
							Queries: config.TideQueries{
								{
									Milestone:        "different",
									Repos:            []string{fmt.Sprintf("%s/%s", testOrg, testRepo)},
									IncludedBranches: []string{testReleaseBranch, defaultBranches[0]},
								},
							},
						},
					},
				},
			},
			isInCodeFreezte: false,
		},
		{
			name: "different repo",
			config: &config.Config{
				ProwConfig: config.ProwConfig{
					Tide: config.Tide{
						TideGitHubConfig: config.TideGitHubConfig{
							Queries: config.TideQueries{
								{
									Milestone:        testMilestone,
									Repos:            []string{"bar"},
									IncludedBranches: []string{testReleaseBranch, defaultBranches[0]},
								},
							},
						},
					},
				},
			},
			isInCodeFreezte: false,
		},
		{
			name: "different included branches",
			config: &config.Config{
				ProwConfig: config.ProwConfig{
					Tide: config.Tide{
						TideGitHubConfig: config.TideGitHubConfig{
							Queries: config.TideQueries{
								{
									Milestone:        testMilestone,
									Repos:            []string{fmt.Sprintf("%s/%s", testOrg, testRepo)},
									IncludedBranches: []string{"some", "other", "branches"},
								},
							},
						},
					},
				},
			},
			isInCodeFreezte: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := New().InCodeFreeze(tc.config, testMilestone, testOrg, testRepo)
			assert.Equal(t, tc.isInCodeFreezte, res)
		})
	}
}
