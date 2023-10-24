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

package phased

import (
	"fmt"
	"errors"
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	utilpointer "k8s.io/utils/pointer"
)

func TestGetPresubmits(t *testing.T) {
	const orgRepo = "my-org/my-repo"

	testCases := []struct {
		name string
		cfg  *config.Config

		expectedPresubmits sets.Set[string]
	}{
		{
			name: "Result of GetPresubmits is used by default",
			cfg: &config.Config{
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						orgRepo: {{
							JobBase: config.JobBase{Name: "my-static-presubmit"},
						}},
					},
					ProwYAMLGetterWithDefaults: func(_ *config.Config, _ git.ClientFactory, _, _ string, _ ...string) (*config.ProwYAML, error) {
						return &config.ProwYAML{
							Presubmits: []config.Presubmit{{
								JobBase: config.JobBase{Name: "my-inrepoconfig-presubmit"},
							}},
						}, nil
					},
				},
				ProwConfig: config.ProwConfig{
					InRepoConfig: config.InRepoConfig{Enabled: map[string]*bool{"*": utilpointer.Bool(true)}},
				},
			},

			expectedPresubmits: sets.New[string]("my-inrepoconfig-presubmit", "my-static-presubmit"),
		},
		{
			name: "Fallback to static presubmits",
			cfg: &config.Config{
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						orgRepo: {{
							JobBase: config.JobBase{Name: "my-static-presubmit"},
						}},
					},
					ProwYAMLGetterWithDefaults: func(_ *config.Config, _ git.ClientFactory, _, _ string, _ ...string) (*config.ProwYAML, error) {
						return &config.ProwYAML{
							Presubmits: []config.Presubmit{{
								JobBase: config.JobBase{Name: "my-inrepoconfig-presubmit"},
							}},
						}, errors.New("some error")
					},
				},
				ProwConfig: config.ProwConfig{
					InRepoConfig: config.InRepoConfig{Enabled: map[string]*bool{"*": utilpointer.Bool(true)}},
				},
			},

			expectedPresubmits: sets.New[string]("my-static-presubmit"),
		},
	}

	shaGetter := func() (string, error) {
		return "", nil
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			presubmits := getPresubmits(logrus.NewEntry(logrus.New()), nil, tc.cfg, orgRepo, shaGetter, shaGetter)
			actualPresubmits := sets.Set[string]{}
			for _, presubmit := range presubmits {
				actualPresubmits.Insert(presubmit.Name)
			}

			if !tc.expectedPresubmits.Equal(actualPresubmits) {
				t.Errorf("got a different set of presubmits than expected, diff: %v", tc.expectedPresubmits.Difference(actualPresubmits))
			}
		})
	}
}

func issueLabels(labels ...string) []string {
	var ls []string
	for _, label := range labels {
		ls = append(ls, fmt.Sprintf("org/repo#0:%s", label))
	}
	return ls
}
