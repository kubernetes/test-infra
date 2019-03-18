/*
Copyright 2017 The Kubernetes Authors.

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

package docsnoretest

import (
	"errors"
	"testing"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/scallywag"
)

type ghc struct {
	*testing.T
	labels    sets.String
	prChanges []scallywag.PullRequestChange

	addLabelErr, removeLabelErr, getIssueLabelsErr, getPullRequestChangesErr error
}

func (c *ghc) AddLabel(_, _ string, _ int, targetLabel string) error {
	c.T.Logf("AddLabel: %s", targetLabel)
	c.labels.Insert(targetLabel)

	return c.addLabelErr
}

func (c *ghc) RemoveLabel(_, _ string, _ int, targetLabel string) error {
	c.T.Logf("RemoveLabel: %s", targetLabel)
	c.labels.Delete(targetLabel)

	return c.removeLabelErr
}

func (c *ghc) GetIssueLabels(_, _ string, _ int) (ls []scallywag.Label, err error) {
	c.T.Log("GetIssueLabels")
	for label := range c.labels {
		ls = append(ls, scallywag.Label{Name: label})
	}

	err = c.getIssueLabelsErr
	return
}

func (c *ghc) GetPullRequestChanges(_, _ string, _ int) ([]scallywag.PullRequestChange, error) {
	c.T.Log("GetPullRequestChanges")
	return c.prChanges, c.getPullRequestChangesErr
}

func TestHandlePR(t *testing.T) {
	var (
		testError = errors.New("test error")
	)

	cases := []struct {
		name             string
		labels           sets.String
		prChanges        []scallywag.PullRequestChange
		err              error
		shouldSkipRetest bool
		action           scallywag.PullRequestEventAction
		addLabelErr, removeLabelErr, getIssueLabelsErr,
		getPullRequestChangesErr error
	}{
		// does not initially have label
		{
			name:   "change md, no label, needs label",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/README.md",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "change svg, no label, needs label",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/graph.svg",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "change OWNERS, no label, needs label",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/OWNERS",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "change LICENSE, no label, needs label",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/LICENSE",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "change SECURITY_CONTACTS, no label, needs label",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/SECURITY_CONTACTS",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "change OWNERS_ALIASES, no label, needs label",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/OWNERS_ALIASES",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "change non doc, no label, needs no label",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/foo.go",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: false,
		},
		{
			name:   "change mix, no label, needs label",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/foo.go",
				},
				{
					Filename: "/path/to/file/foo.md",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: false,
		},
		// initially has label
		{
			name:   "change md, has label, needs label",
			labels: sets.NewString(labelSkipRetest),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/README.md",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "change svg, has label, needs label",
			labels: sets.NewString(labelSkipRetest),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/graph.svg",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "change OWNERS, has label, needs label",
			labels: sets.NewString(labelSkipRetest),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/OWNERS",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "change LICENSE, has label, needs label",
			labels: sets.NewString(labelSkipRetest),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/LICENSE",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "change SECURITY_CONTACTS, has label, needs label",
			labels: sets.NewString(labelSkipRetest),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/SECURITY_CONTACTS",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "change OWNERS_ALIASES, has label, needs label",
			labels: sets.NewString(labelSkipRetest),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/OWNERS_ALIASES",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "change non doc, has label, needs no label",
			labels: sets.NewString(labelSkipRetest),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/foo.go",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: false,
		},
		{
			name:   "change mix, has label, needs label",
			labels: sets.NewString(labelSkipRetest),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/foo.go",
				},
				{
					Filename: "/path/to/file/foo.md",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: false,
		},
		// check action
		{
			name:   "action opened",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/foo.md",
				},
			},
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "action reopened",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/foo.md",
				},
			},
			action:           scallywag.PullRequestActionReopened,
			shouldSkipRetest: true,
		},
		{
			name:   "action synchronize",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/foo.md",
				},
			},
			action:           scallywag.PullRequestActionSynchronize,
			shouldSkipRetest: true,
		},
		{
			name:   "action closed",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/foo.md",
				},
			},
			action:           scallywag.PullRequestActionClosed,
			shouldSkipRetest: false, // since it is closed, should not change
		},
		// error handling
		{
			name:   "error getting pull request changes",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/foo.go",
				},
			},
			getPullRequestChangesErr: testError,
			err:                      testError,
			action:                   scallywag.PullRequestActionOpened,
			shouldSkipRetest:         false,
		},
		{
			name:   "error getting labels",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/foo.go",
				},
			},
			getIssueLabelsErr: testError,
			err:               testError,
			action:            scallywag.PullRequestActionOpened,
			shouldSkipRetest:  false,
		},
		{
			name:   "error adding label",
			labels: sets.NewString(),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/foo.md",
				},
			},
			addLabelErr:      testError,
			err:              testError,
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: true,
		},
		{
			name:   "error removing label",
			labels: sets.NewString(labelSkipRetest),
			prChanges: []scallywag.PullRequestChange{
				{
					Filename: "/path/to/file/foo.go",
				},
			},
			removeLabelErr:   testError,
			err:              testError,
			action:           scallywag.PullRequestActionOpened,
			shouldSkipRetest: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client := &ghc{
				labels:                   c.labels,
				prChanges:                c.prChanges,
				addLabelErr:              c.addLabelErr,
				removeLabelErr:           c.removeLabelErr,
				getIssueLabelsErr:        c.getIssueLabelsErr,
				getPullRequestChangesErr: c.getPullRequestChangesErr,
				T:                        t,
			}

			event := scallywag.PullRequestEvent{
				Action: c.action,
				Number: 101,
				PullRequest: scallywag.PullRequest{
					Number: 101,
					Base: scallywag.PullRequestBranch{
						SHA: "abcd",
						Repo: scallywag.Repo{
							Owner: scallywag.User{
								Login: "kubernetes",
							},
							Name: "kubernetes",
						},
					},
				},
			}

			err := handlePR(client, event)

			if err != nil && c.err == nil {
				t.Errorf("test case \"%s\": unexpected handlePR error: %v", c.name, err)
			}

			if err == nil && c.err != nil {
				t.Errorf("test case \"%s\": handlePR wanted error %v, got nil", c.name, err)
			}

			if !client.labels.Has(labelSkipRetest) && c.shouldSkipRetest {
				t.Errorf("test case \"%s\": github client is missing expected label %s", c.name, labelSkipRetest)
			} else if client.labels.Has(labelSkipRetest) && !c.shouldSkipRetest {
				t.Errorf("test case \"%s\": github client unexpectedly has label %s", c.name, labelSkipRetest)
			}
		})
	}
}
