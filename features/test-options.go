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

package features

import (
	"k8s.io/contrib/mungegithub/github"

	"github.com/spf13/cobra"
)

const (
	// TestOptionsFeature is how mungers should indicate this is required.
	TestOptionsFeature = "test-options"
)

// TestOptions is a struct that handles parameters required by mungers
// to find out about specific tests.
type TestOptions struct {
	RequiredRetestContexts []string
}

func init() {
	RegisterFeature(&TestOptions{})
}

// Name is just going to return the name mungers use to request this feature
func (t *TestOptions) Name() string {
	return TestOptionsFeature
}

// Initialize will initialize the feature.
func (t *TestOptions) Initialize(config *github.Config) error {
	return nil
}

// EachLoop is called at the start of every munge loop
func (t *TestOptions) EachLoop() error {
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (t *TestOptions) AddFlags(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(&t.RequiredRetestContexts, "required-retest-contexts", []string{}, "Comma separate list of statuses which will be retested and which must come back green after the `retest-body` comment is posted to a PR")
}
