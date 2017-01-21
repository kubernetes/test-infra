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
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
)

const (
	// BranchProtectionFeature should update the branches with the required contexts
	BranchProtectionFeature = "branch-protection"
)

// BranchProtection is a features that sets branches as protected
type BranchProtection struct {
	cmd           *cobra.Command
	config        *github.Config
	branches      []string
	extraContexts []string
}

func init() {
	bp := BranchProtection{}
	RegisterFeature(&bp)
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
	cmd := bp.cmd
	contexts := []string{}
	contexts = append(contexts, bp.extraContexts...)

	if c, err := cmd.Flags().GetStringSlice("required-contexts"); err != nil {
		glog.Errorf("unable to get flag `required-contexts`: %v", err)
	} else {
		contexts = append(contexts, c...)
	}

	if c, err := cmd.Flags().GetStringSlice("required-retest-contexts"); err != nil {
		glog.Errorf("unable to get flag `required-retest-contexts`: %v", err)
	} else {
		contexts = append(contexts, c...)
	}

	for _, branch := range bp.branches {
		bp.config.SetBranchProtection(branch, contexts)
	}
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (bp *BranchProtection) AddFlags(cmd *cobra.Command) {
	bp.cmd = cmd
	cmd.Flags().StringSliceVar(&bp.branches, "protected-branches", []string{}, "branches to be marked 'protected'.  required-contexts, required-retest-contexts, and protected-branches-extra-contexts will be marked as required for non-admins")
	cmd.Flags().StringSliceVar(&bp.extraContexts, "protected-branches-extra-contexts", []string{}, "Contexts which will be marked as required in the Github UI but which the bot itself does not require")
}
