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
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
)

var defaultBranches = []string{"master", "main"}

// CodeFreezeChecker is the main structure of checking if we're in Code Freeze.
type CodeFreezeChecker struct{}

// New creates a new CodeFreezeChecker instance.
func New() *CodeFreezeChecker {
	return &CodeFreezeChecker{}
}

// InCodeFreeze returns true if we're in Code Freeze:
// https://github.com/kubernetes/sig-release/blob/2d8a1cc/releases/release_phases.md#code-freeze
// This is being checked if the prow tide configuration has milestone restrictions applied like:
// https://github.com/kubernetes/test-infra/pull/31164/files
func (c *CodeFreezeChecker) InCodeFreeze(prowConfig *config.Config, milestone, org, repo string) bool {
	orgRepo := config.OrgRepo{Org: org, Repo: repo}
	queries := prowConfig.Tide.Queries.QueryMap().ForRepo(orgRepo)

	for _, query := range queries {
		if query.Milestone != milestone {
			continue
		}

		includedBranches := sets.New(query.IncludedBranches...)
		releaseBranchFromMilestone := fmt.Sprintf("release-%s", strings.TrimPrefix(query.Milestone, "v"))

		if includedBranches.Has(releaseBranchFromMilestone) && includedBranches.HasAny(defaultBranches...) {
			logrus.Infof("Found code freeze for milestone %s", query.Milestone)
			return true
		}
	}

	return false
}
