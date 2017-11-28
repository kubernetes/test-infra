/*
Copyright 2016 The Kubernetes Authors.

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

package features

import (
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/mungegithub/github"
	"k8s.io/test-infra/mungegithub/mungeopts"
	"k8s.io/test-infra/mungegithub/options"
)

const (
	// BranchProtectionFeature should update the branches with the required contexts
	BranchProtectionFeature = "branch-protection"
)

// BranchProtection is a features that sets branches as protected
type BranchProtection struct {
	config *github.Config

	branches      []string
	extraContexts []string
}

func init() {
	RegisterFeature(&BranchProtection{})
}

// Name is just going to return the name mungers use to request this feature
func (bp *BranchProtection) Name() string {
	return BranchProtectionFeature
}

// Initialize will initialize the feature.
func (bp *BranchProtection) Initialize(config *github.Config) error {
	bp.config = config
	return nil
}

// EachLoop is called at the start of every munge loop
func (bp *BranchProtection) EachLoop() error {
	contexts := []string{}
	contexts = append(contexts, bp.extraContexts...)
	contexts = append(contexts, mungeopts.RequiredContexts.Merge...)
	contexts = append(contexts, mungeopts.RequiredContexts.Retest...)

	for _, branch := range bp.branches {
		bp.config.SetBranchProtection(branch, contexts)
	}
	return nil
}

// RegisterOptions registers options for this feature; returns any that require a restart when changed.
func (bp *BranchProtection) RegisterOptions(opts *options.Options) sets.String {
	opts.RegisterStringSlice(&bp.branches, "protected-branches", []string{}, "branches to be marked 'protected'.  required-contexts, required-retest-contexts, and protected-branches-extra-contexts will be marked as required for non-admins")
	opts.RegisterStringSlice(&bp.extraContexts, "protected-branches-extra-contexts", []string{}, "Contexts which will be marked as required in the Github UI but which the bot itself does not require")
	return nil
}
