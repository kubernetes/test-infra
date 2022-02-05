/*
Copyright 2018 The Kubernetes Authors.

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

package trigger

import (
	"fmt"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	clienttesting "k8s.io/client-go/testing"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/diff"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"

	"k8s.io/test-infra/prow/github/fakegithub"
)

func TestCreateRefs(t *testing.T) {
	pe := github.PushEvent{
		Ref: "refs/heads/master",
		Repo: github.Repo{
			Owner: github.User{
				Name: "kubernetes",
			},
			Name:    "repo",
			HTMLURL: "https://example.com/kubernetes/repo",
		},
		After:   "abcdef",
		Compare: "https://example.com/kubernetes/repo/compare/abcdee...abcdef",
	}
	expected := prowapi.Refs{
		Org:      "kubernetes",
		Repo:     "repo",
		RepoLink: "https://example.com/kubernetes/repo",
		BaseRef:  "master",
		BaseSHA:  "abcdef",
		BaseLink: "https://example.com/kubernetes/repo/compare/abcdee...abcdef",
	}
	if actual := createRefs(pe); !equality.Semantic.DeepEqual(expected, actual) {
		t.Errorf("diff between expected and actual refs:%s", diff.ObjectReflectDiff(expected, actual))
	}
}

func getProwRefs(num int) *prowapi.Refs {
	return &prowapi.Refs{
		Org:      "org2",
		Repo:     "repo2",
		RepoLink: "HTMLURL",
		BaseRef:  "master",
		BaseSHA:  "SHA",
		BaseLink: "HTMLURL/commit/SHA",
		Author:   "author",
		Pulls: []prowapi.Pull{
			{
				Number:     num,
				Author:     "author",
				SHA:        "HEAD",
				Link:       "PRURL",
				AuthorLink: "authorURL",
				CommitLink: fmt.Sprintf("HTMLURL/pull/%d/commits/HEAD", num),
			},
		},
	}
}

func getPR(num int, mergedAt time.Time) github.PullRequest {
	return github.PullRequest{
		Number: num,
		User: github.User{
			Login:   "author",
			HTMLURL: "authorURL",
		},
		Base: github.PullRequestBranch{
			Repo: github.Repo{
				Name:    "repo2",
				Owner:   github.User{Login: "org2"},
				HTMLURL: "HTMLURL",
			},
			Ref: "master",
			SHA: "SHA",
		},
		Head: github.PullRequestBranch{
			SHA: "HEAD",
		},
		HTMLURL:  "PRURL",
		MergedAt: mergedAt,
	}
}

func TestHandlePE(t *testing.T) {
	now := time.Now()
	testCases := []struct {
		name         string
		pe           github.PushEvent
		commitPulls  map[string][]github.PullRequest
		jobsToRun    int
		expectedRefs *prowapi.Refs
	}{
		{
			name: "branch deleted",
			pe: github.PushEvent{
				Ref: "refs/heads/master",
				Repo: github.Repo{
					Owner: github.User{Login: "org"},
					Name:  "repo",
				},
				Deleted: true,
			},
			jobsToRun: 0,
		},
		{
			name: "null after sha",
			pe: github.PushEvent{
				After: "0000000000000000000000000000000000000000",
				Ref:   "refs/heads/master",
				Repo: github.Repo{
					Owner: github.User{Login: "org"},
					Name:  "repo",
				},
			},
			jobsToRun: 0,
		},
		{
			name: "no matching files",
			pe: github.PushEvent{
				Ref: "refs/heads/master",
				Commits: []github.Commit{
					{
						Added: []string{"example.txt"},
					},
				},
				Repo: github.Repo{
					Owner: github.User{Login: "org"},
					Name:  "repo",
				},
			},
		},
		{
			name: "one matching file",
			pe: github.PushEvent{
				Ref: "refs/heads/master",
				Commits: []github.Commit{
					{
						Added:    []string{"example.txt"},
						Modified: []string{"hack.sh"},
					},
				},
				Repo: github.Repo{
					Owner: github.User{Login: "org"},
					Name:  "repo",
				},
			},
			jobsToRun: 1,
		},
		{
			name: "no change matcher",
			pe: github.PushEvent{
				Ref: "refs/heads/master",
				Commits: []github.Commit{
					{
						Added: []string{"example.txt"},
					},
				},
				Repo: github.Repo{
					Owner: github.User{Login: "org2"},
					Name:  "repo2",
				},
			},
			jobsToRun: 1,
		},
		{
			name: "branch name with a slash",
			pe: github.PushEvent{
				Ref: "refs/heads/release/v1.14",
				Commits: []github.Commit{
					{
						Added: []string{"hack.sh"},
					},
				},
				Repo: github.Repo{
					Owner: github.User{Login: "org3"},
					Name:  "repo3",
				},
			},
			jobsToRun: 1,
		},
		{
			name: "postsubmit job gets ref to the most recently merged PR associated with commits",
			pe: github.PushEvent{
				Ref:   "refs/heads/master",
				After: "SHA",
				Commits: []github.Commit{
					{
						ID:    "SHA1",
						Added: []string{"example1.txt"},
					},
					{
						ID:    "SHA2",
						Added: []string{"example2.txt"},
					},
				},
				Repo: github.Repo{
					Owner: github.User{Login: "org2"},
					Name:  "repo2",
				},
			},
			commitPulls: map[string][]github.PullRequest{
				"org2/repo2/SHA1": {
					getPR(1, now),
					getPR(2, now.AddDate(2, 0, 0)),
					getPR(3, now),
				},
			},
			jobsToRun:    1,
			expectedRefs: getProwRefs(2),
		},
		{
			name: "unmerged PRs associated with commits are skipped when assigning ref to postsubmit job",
			pe: github.PushEvent{
				Ref:   "refs/heads/master",
				After: "SHA",
				Commits: []github.Commit{
					{
						ID:    "SHA1",
						Added: []string{"example1.txt"},
					},
					{
						ID:    "SHA2",
						Added: []string{"example2.txt"},
					},
				},
				Repo: github.Repo{
					Owner: github.User{Login: "org2"},
					Name:  "repo2",
				},
			},
			commitPulls: map[string][]github.PullRequest{
				"org2/repo2/SHA2": {
					getPR(3, time.Time{}),
					getPR(4, now.AddDate(1, 0, 0)),
					getPR(5, time.Time{}),
				},
			},
			jobsToRun:    1,
			expectedRefs: getProwRefs(4),
		},
		{
			name: "no PR refs set when all PRs are unmerged",
			pe: github.PushEvent{
				Ref:    "refs/heads/master",
				After:  "SHA",
				Sender: github.User{Login: "author"},
				Commits: []github.Commit{
					{
						ID:    "SHA1",
						Added: []string{"example1.txt"},
					},
					{
						ID:    "SHA2",
						Added: []string{"example2.txt"},
					},
				},
				Repo: github.Repo{
					Owner: github.User{Login: "org2", Name: "org2"},
					Name:  "repo2",
				},
			},
			commitPulls: map[string][]github.PullRequest{
				"org2/repo2/SHA1": {
					getPR(1, time.Time{}),
					getPR(2, time.Time{}),
				},
			},
			jobsToRun: 1,
			expectedRefs: &prowapi.Refs{
				Org:     "org2",
				Repo:    "repo2",
				BaseRef: "master",
				BaseSHA: "SHA",
				Author:  "author",
			},
		},
		{
			name: "no PR refs set when postsubmit's comment_on field is not set",
			pe: github.PushEvent{
				Ref:    "refs/heads/master",
				After:  "SHA",
				Sender: github.User{Login: "author"},
				Commits: []github.Commit{
					{
						ID:    "SHA1",
						Added: []string{"triggers-run-if-changed-postsubmit.sh"},
					},
				},
				Repo: github.Repo{
					Owner: github.User{Name: "org4", Login: "org4"},
					Name:  "repo4",
				},
			},
			commitPulls: map[string][]github.PullRequest{
				"org4/repo4/SHA1": {
					getPR(1, now),
				},
			},
			jobsToRun: 1,
			expectedRefs: &prowapi.Refs{
				Org:     "org4",
				Repo:    "repo4",
				BaseRef: "master",
				BaseSHA: "SHA",
				Author:  "author",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := fakegithub.NewFakeClient()
			g.CommitPullRequests = tc.commitPulls
			fakeProwJobClient := fake.NewSimpleClientset()
			c := Client{
				GitHubClient:  g,
				ProwJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
				Config:        &config.Config{ProwConfig: config.ProwConfig{ProwJobNamespace: "prowjobs"}},
				Logger:        logrus.WithField("plugin", PluginName),
			}
			postsubmits := map[string][]config.Postsubmit{
				"org/repo": {
					{
						JobBase: config.JobBase{
							Name: "pass-butter",
						},
						RegexpChangeMatcher: config.RegexpChangeMatcher{
							RunIfChanged: "\\.sh$",
						},
					},
				},
				"org2/repo2": {
					{
						JobBase: config.JobBase{
							Name: "pass-salt",
							ReporterConfig: &prowapi.ReporterConfig{
								GitHub: &prowapi.GitHubReporterConfig{
									CommentOnPostsubmits: true,
								},
							},
						},
					},
					{
						JobBase: config.JobBase{
							Name: "pass-butter",
						},
						RegexpChangeMatcher: config.RegexpChangeMatcher{
							RunIfChanged: "\\.sh$",
						},
					},
				},
				"org3/repo3": {
					{
						JobBase: config.JobBase{
							Name: "pass-pepper",
						},
						Brancher: config.Brancher{
							Branches: []string{"release/v1.14"},
						},
					},
				},
				"org4/repo4": {
					{
						JobBase: config.JobBase{
							Name: "pass-butter",
						},
						RegexpChangeMatcher: config.RegexpChangeMatcher{
							RunIfChanged: "\\.sh$",
						},
					},
				},
			}
			if err := c.Config.SetPostsubmits(postsubmits); err != nil {
				t.Fatalf("failed to set postsubmits: %v", err)
			}
			err := handlePE(c, tc.pe)
			if err != nil {
				t.Errorf("test %q: handlePE returned unexpected error %v", tc.name, err)
			}
			var created []*prowapi.ProwJob
			for _, action := range fakeProwJobClient.Fake.Actions() {
				switch action := action.(type) {
				case clienttesting.CreateActionImpl:
					if prowjob, ok := action.Object.(*prowapi.ProwJob); ok {
						created = append(created, prowjob)
					}
				}
			}
			if len(created) != tc.jobsToRun {
				t.Fatalf("test %q: expected %d jobs to run, got %d", tc.name, tc.jobsToRun, len(created))
			}
			if tc.jobsToRun > 0 && tc.expectedRefs != nil {
				if !equality.Semantic.DeepEqual(tc.expectedRefs, created[0].Spec.Refs) {
					t.Errorf("diff between expected and actual refs:%s", diff.ObjectReflectDiff(tc.expectedRefs, created[0].Spec.Refs))
				}
			}
		})
	}
}
