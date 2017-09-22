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

package size

import (
	"testing"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
)

type ghc struct {
	*testing.T
	labels    map[github.Label]bool
	files     map[string][]byte
	prChanges []github.PullRequestChange

	addLabelErr, removeLabelErr, getIssueLabelsErr,
	getFileErr, getPullRequestChangesErr error
}

func (c *ghc) AddLabel(_, _ string, _ int, label string) error {
	c.T.Logf("AddLabel: %s", label)
	c.labels[github.Label{Name: label}] = true

	return c.addLabelErr
}

func (c *ghc) RemoveLabel(_, _ string, _ int, label string) error {
	c.T.Logf("RemoveLabel: %s", label)
	for k := range c.labels {
		if k.Name == label {
			delete(c.labels, k)
		}
	}

	return c.removeLabelErr
}

func (c *ghc) GetIssueLabels(_, _ string, _ int) (ls []github.Label, err error) {
	c.T.Log("GetIssueLabels")
	for k, ok := range c.labels {
		if ok {
			ls = append(ls, k)
		}
	}

	err = c.getIssueLabelsErr
	return
}

func (c *ghc) GetFile(_, _, path, _ string) ([]byte, error) {
	c.T.Logf("GetFile: %s", path)
	return c.files[path], c.getFileErr
}

func (c *ghc) GetPullRequestChanges(_, _ string, _ int) ([]github.PullRequestChange, error) {
	c.T.Log("GetPullRequestChanges")
	return c.prChanges, c.getPullRequestChangesErr
}

func TestHandlePR(t *testing.T) {
	cases := []struct {
		name        string
		client      *ghc
		event       github.PullRequestEvent
		err         error
		finalLabels []github.Label
	}{
		{
			name: "simple size/S, no .generated_files",
			client: &ghc{
				labels:     map[github.Label]bool{},
				getFileErr: &github.FileNotFound{},
				prChanges: []github.PullRequestChange{
					{
						SHA:       "abcd",
						Filename:  "foobar",
						Additions: 10,
						Deletions: 10,
						Changes:   20,
					},
					{
						SHA:       "abcd",
						Filename:  "barfoo",
						Additions: 3,
						Deletions: 4,
						Changes:   7,
					},
				},
			},
			event: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				Number: 101,
				PullRequest: github.PullRequest{
					Number: 101,
					Base: github.PullRequestBranch{
						SHA: "abcd",
						Repo: github.Repo{
							Owner: github.User{
								Login: "kubernetes",
							},
							Name: "kubernetes",
						},
					},
				},
			},
			finalLabels: []github.Label{
				{Name: "size/S"},
			},
		},
		{
			name: "simple size/M, with .generated_files",
			client: &ghc{
				labels: map[github.Label]bool{},
				files: map[string][]byte{
					".generated_files": []byte(`
						file-name foobar

						path-prefix generated
					`),
				},
				prChanges: []github.PullRequestChange{
					{
						SHA:       "abcd",
						Filename:  "foobar",
						Additions: 10,
						Deletions: 10,
						Changes:   20,
					},
					{
						SHA:       "abcd",
						Filename:  "barfoo",
						Additions: 50,
						Deletions: 0,
						Changes:   50,
					},
					{
						SHA:       "abcd",
						Filename:  "generated/what.txt",
						Additions: 30,
						Deletions: 0,
						Changes:   30,
					},
					{
						SHA:       "abcd",
						Filename:  "generated/my/file.txt",
						Additions: 300,
						Deletions: 0,
						Changes:   300,
					},
				},
			},
			event: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				Number: 101,
				PullRequest: github.PullRequest{
					Number: 101,
					Base: github.PullRequestBranch{
						SHA: "abcd",
						Repo: github.Repo{
							Owner: github.User{
								Login: "kubernetes",
							},
							Name: "kubernetes",
						},
					},
				},
			},
			finalLabels: []github.Label{
				{Name: "size/M"},
			},
		},
		{
			name: "simple size/XS, with .generated_files and paths-from-repo",
			client: &ghc{
				labels: map[github.Label]bool{},
				files: map[string][]byte{
					".generated_files": []byte(`
						# Comments
						file-name foobar

						path-prefix generated

						paths-from-repo docs/.generated_docs
					`),
					"docs/.generated_docs": []byte(`
					# Comments work

					# And empty lines don't matter
					foobar
					mypath1
					mypath2
					mydir/mypath3
					`),
				},
				prChanges: []github.PullRequestChange{
					{
						SHA:       "abcd",
						Filename:  "foobar",
						Additions: 10,
						Deletions: 10,
						Changes:   20,
					},
					{ // Notice "barfoo" is the only relevant change.
						SHA:       "abcd",
						Filename:  "barfoo",
						Additions: 5,
						Deletions: 0,
						Changes:   5,
					},
					{
						SHA:       "abcd",
						Filename:  "generated/what.txt",
						Additions: 30,
						Deletions: 0,
						Changes:   30,
					},
					{
						SHA:       "abcd",
						Filename:  "generated/my/file.txt",
						Additions: 300,
						Deletions: 0,
						Changes:   300,
					},
					{
						SHA:       "abcd",
						Filename:  "mypath1",
						Additions: 300,
						Deletions: 0,
						Changes:   300,
					},
					{
						SHA:       "abcd",
						Filename:  "mydir/mypath3",
						Additions: 300,
						Deletions: 0,
						Changes:   300,
					},
				},
			},
			event: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				Number: 101,
				PullRequest: github.PullRequest{
					Number: 101,
					Base: github.PullRequestBranch{
						SHA: "abcd",
						Repo: github.Repo{
							Owner: github.User{
								Login: "kubernetes",
							},
							Name: "kubernetes",
						},
					},
				},
			},
			finalLabels: []github.Label{
				{Name: "size/XS"},
			},
		},
		{
			name:   "pr closed event",
			client: &ghc{},
			event: github.PullRequestEvent{
				Action: github.PullRequestActionClosed,
			},
			finalLabels: []github.Label{},
		},
		{
			name: "XS -> S transition",
			client: &ghc{
				labels: map[github.Label]bool{
					{Name: "irrelevant"}: true,
					{Name: "size/XS"}:    true,
				},
				files: map[string][]byte{
					".generated_files": []byte(`
						# Comments
						file-name foobar

						path-prefix generated

						paths-from-repo docs/.generated_docs
					`),
					"docs/.generated_docs": []byte(`
					# Comments work

					# And empty lines don't matter
					foobar
					mypath1
					mypath2
					mydir/mypath3
					`),
				},
				prChanges: []github.PullRequestChange{
					{
						SHA:       "abcd",
						Filename:  "foobar",
						Additions: 10,
						Deletions: 10,
						Changes:   20,
					},
					{ // Notice "barfoo" is the only relevant change.
						SHA:       "abcd",
						Filename:  "barfoo",
						Additions: 5,
						Deletions: 0,
						Changes:   5,
					},
					{
						SHA:       "abcd",
						Filename:  "generated/what.txt",
						Additions: 30,
						Deletions: 0,
						Changes:   30,
					},
					{
						SHA:       "abcd",
						Filename:  "generated/my/file.txt",
						Additions: 300,
						Deletions: 0,
						Changes:   300,
					},
					{
						SHA:       "abcd",
						Filename:  "mypath1",
						Additions: 300,
						Deletions: 0,
						Changes:   300,
					},
					{
						SHA:       "abcd",
						Filename:  "mydir/mypath3",
						Additions: 300,
						Deletions: 0,
						Changes:   300,
					},
				},
			},
			event: github.PullRequestEvent{
				Action: github.PullRequestActionOpened,
				Number: 101,
				PullRequest: github.PullRequest{
					Number: 101,
					Base: github.PullRequestBranch{
						SHA: "abcd",
						Repo: github.Repo{
							Owner: github.User{
								Login: "kubernetes",
							},
							Name: "kubernetes",
						},
					},
				},
			},
			finalLabels: []github.Label{
				{Name: "irrelevant"},
				{Name: "size/XS"},
			},
		},
		{
			name: "pull request reopened",
			client: &ghc{
				labels:     map[github.Label]bool{},
				getFileErr: &github.FileNotFound{},
				prChanges: []github.PullRequestChange{
					{
						SHA:       "abcd",
						Filename:  "foobar",
						Additions: 10,
						Deletions: 10,
						Changes:   20,
					},
					{
						SHA:       "abcd",
						Filename:  "barfoo",
						Additions: 3,
						Deletions: 4,
						Changes:   7,
					},
				},
			},
			event: github.PullRequestEvent{
				Action: github.PullRequestActionReopened,
				Number: 101,
				PullRequest: github.PullRequest{
					Number: 101,
					Base: github.PullRequestBranch{
						SHA: "abcd",
						Repo: github.Repo{
							Owner: github.User{
								Login: "kubernetes",
							},
							Name: "kubernetes",
						},
					},
				},
			},
			finalLabels: []github.Label{
				{Name: "size/S"},
			},
		},
		{
			name: "pull request edited",
			client: &ghc{
				labels:     map[github.Label]bool{},
				getFileErr: &github.FileNotFound{},
				prChanges: []github.PullRequestChange{
					{
						SHA:       "abcd",
						Filename:  "foobar",
						Additions: 30,
						Deletions: 40,
						Changes:   70,
					},
				},
			},
			event: github.PullRequestEvent{
				Action: github.PullRequestActionEdited,
				Number: 101,
				PullRequest: github.PullRequest{
					Number: 101,
					Base: github.PullRequestBranch{
						SHA: "abcd",
						Repo: github.Repo{
							Owner: github.User{
								Login: "kubernetes",
							},
							Name: "kubernetes",
						},
					},
				},
			},
			finalLabels: []github.Label{
				{Name: "size/M"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.client == nil {
				t.Fatalf("case can not have nil github client")
			}

			// Set up test logging.
			c.client.T = t

			err := handlePR(c.client, logrus.NewEntry(logrus.New()), c.event)

			if err != nil && c.err == nil {
				t.Fatalf("handlePR error: %v", err)
			}

			if err == nil && c.err != nil {
				t.Fatalf("handlePR wanted error %v, got nil", err)
			}

			if got, want := err, c.err; got != nil && got.Error() != want.Error() {
				t.Fatalf("handlePR errors mismatch: got %v, want %v", got, want)
			}

			if got, want := len(c.client.labels), len(c.finalLabels); got != want {
				t.Logf("github client labels: got %v; want %v", c.client.labels, c.finalLabels)
				t.Fatalf("finalLabels count mismatch: got %d, want %d", got, want)
			}

			for _, l := range c.finalLabels {
				if !c.client.labels[l] {
					t.Fatalf("github client labels missing %v", l)
				}
			}
		})
	}
}
