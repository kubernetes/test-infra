/*
Copyright 2019 The Kubernetes Authors.

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
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sirupsen/logrus"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	clienttesting "k8s.io/client-go/testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/plugins"
	utilpointer "k8s.io/utils/pointer"
)

func TestHelpProvider(t *testing.T) {
	enabledRepos := []config.OrgRepo{
		{Org: "org1", Repo: "repo"},
		{Org: "org2", Repo: "repo"},
	}
	cases := []struct {
		name         string
		config       *plugins.Configuration
		enabledRepos []config.OrgRepo
		err          bool
	}{
		{
			name:         "Empty config",
			config:       &plugins.Configuration{},
			enabledRepos: enabledRepos,
		},
		{
			name: "All configs enabled",
			config: &plugins.Configuration{
				Triggers: []plugins.Trigger{
					{
						Repos:          []string{"org2/repo"},
						TrustedOrg:     "org2",
						JoinOrgURL:     "https://join.me",
						OnlyOrgMembers: true,
						IgnoreOkToTest: true,
					},
				},
			},
			enabledRepos: enabledRepos,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := helpProvider(c.config, c.enabledRepos)
			if err != nil && !c.err {
				t.Fatalf("helpProvider error: %v", err)
			}
		})
	}
}

func TestRunRequested(t *testing.T) {
	var testCases = []struct {
		name string

		requestedJobs   []config.Presubmit
		jobCreationErrs sets.String // job names which fail creation

		expectedJobs sets.String // by name
		expectedErr  bool
	}{
		{
			name: "nothing requested means nothing done",
		},
		{
			name: "disjoint sets of jobs get triggered",
			requestedJobs: []config.Presubmit{{
				JobBase: config.JobBase{
					Name: "first",
				},
				Reporter: config.Reporter{Context: "first-context"},
			}, {
				JobBase: config.JobBase{
					Name: "second",
				},
				Reporter: config.Reporter{Context: "second-context"},
			}},
			expectedJobs: sets.NewString("first", "second"),
		},
		{
			name: "all requested jobs get run",
			requestedJobs: []config.Presubmit{{
				JobBase: config.JobBase{
					Name: "first",
				},
				Reporter: config.Reporter{Context: "first-context"},
			}, {
				JobBase: config.JobBase{
					Name: "second",
				},
				Reporter: config.Reporter{Context: "second-context"},
			}},
			expectedJobs: sets.NewString("first", "second"),
		},
		{
			name: "failure on job creation bubbles up but doesn't stop others from starting",
			requestedJobs: []config.Presubmit{{
				JobBase: config.JobBase{
					Name: "first",
				},
				Reporter: config.Reporter{Context: "first-context"},
			}, {
				JobBase: config.JobBase{
					Name: "second",
				},
				Reporter: config.Reporter{Context: "second-context"},
			}},
			jobCreationErrs: sets.NewString("first"),
			expectedJobs:    sets.NewString("second"),
			expectedErr:     true,
		},
	}

	pr := &github.PullRequest{
		Base: github.PullRequestBranch{
			Repo: github.Repo{
				Owner: github.User{
					Login: "org",
				},
				Name: "repo",
			},
			Ref: "branch",
		},
		Head: github.PullRequestBranch{
			SHA: "foobar1",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var fakeGitHubClient fakegithub.FakeClient
			fakeProwJobClient := fake.NewSimpleClientset()
			fakeProwJobClient.PrependReactor("*", "*", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				switch action := action.(type) {
				case clienttesting.CreateActionImpl:
					prowJob, ok := action.Object.(*prowapi.ProwJob)
					if !ok {
						return false, nil, nil
					}
					if testCase.jobCreationErrs.Has(prowJob.Spec.Job) {
						return true, action.Object, errors.New("failed to create job")
					}
				}
				return false, nil, nil
			})
			client := Client{
				GitHubClient:  &fakeGitHubClient,
				ProwJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
				Logger:        logrus.WithField("testcase", testCase.name),
			}

			err := runRequested(client, pr, fakegithub.TestRef, testCase.requestedJobs, "event-guid", time.Nanosecond)
			if err == nil && testCase.expectedErr {
				t.Error("failed to receive an error")
			}
			if err != nil && !testCase.expectedErr {
				t.Errorf("unexpected error: %v", err)
			}

			observedCreatedProwJobs := sets.NewString()
			existingProwJobs, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").List(context.Background(), metav1.ListOptions{})
			if err != nil {
				t.Errorf("could not list current state of prow jobs: %v", err)
				return
			}
			for _, job := range existingProwJobs.Items {
				observedCreatedProwJobs.Insert(job.Spec.Job)
			}

			if missing := testCase.expectedJobs.Difference(observedCreatedProwJobs); missing.Len() > 0 {
				t.Errorf("didn't create all expected ProwJobs, missing: %s", missing.List())
			}
			if extra := observedCreatedProwJobs.Difference(testCase.expectedJobs); extra.Len() > 0 {
				t.Errorf("created unexpected ProwJobs: %s", extra.List())
			}
		})
	}
}

func TestValidateContextOverlap(t *testing.T) {
	var testCases = []struct {
		name          string
		toRun, toSkip []config.Presubmit
		expectedErr   bool
	}{
		{
			name:   "empty inputs mean no error",
			toRun:  []config.Presubmit{},
			toSkip: []config.Presubmit{},
		},
		{
			name:   "disjoint sets mean no error",
			toRun:  []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}},
			toSkip: []config.Presubmit{{Reporter: config.Reporter{Context: "bar"}}},
		},
		{
			name:   "complex disjoint sets mean no error",
			toRun:  []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "otherfoo"}}},
			toSkip: []config.Presubmit{{Reporter: config.Reporter{Context: "bar"}}, {Reporter: config.Reporter{Context: "otherbar"}}},
		},
		{
			name:        "overlapping sets error",
			toRun:       []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "otherfoo"}}},
			toSkip:      []config.Presubmit{{Reporter: config.Reporter{Context: "bar"}}, {Reporter: config.Reporter{Context: "otherfoo"}}},
			expectedErr: true,
		},
		{
			name:        "identical sets error",
			toRun:       []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "otherfoo"}}},
			toSkip:      []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "otherfoo"}}},
			expectedErr: true,
		},
		{
			name:        "superset callErrors",
			toRun:       []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "otherfoo"}}},
			toSkip:      []config.Presubmit{{Reporter: config.Reporter{Context: "foo"}}, {Reporter: config.Reporter{Context: "otherfoo"}}, {Reporter: config.Reporter{Context: "thirdfoo"}}},
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		validateErr := validateContextOverlap(testCase.toRun, testCase.toSkip)
		if validateErr == nil && testCase.expectedErr {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}
		if validateErr != nil && !testCase.expectedErr {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, validateErr)
		}
	}
}

func TestTrustedUser(t *testing.T) {
	var testcases = []struct {
		name string

		onlyOrgMembers bool
		trustedOrg     string

		user string
		org  string
		repo string

		expectedTrusted bool
		expectedReason  string
	}{
		{
			name:            "user is member of trusted org",
			onlyOrgMembers:  false,
			user:            "test",
			org:             "kubernetes",
			repo:            "kubernetes",
			expectedTrusted: true,
		},
		{
			name:            "user is member of trusted org (only org members enabled)",
			onlyOrgMembers:  true,
			user:            "test",
			org:             "kubernetes",
			repo:            "kubernetes",
			expectedTrusted: true,
		},
		{
			name:            "user is collaborator",
			onlyOrgMembers:  false,
			user:            "test-collaborator",
			org:             "kubernetes",
			repo:            "kubernetes",
			expectedTrusted: true,
		},
		{
			name:            "user is collaborator (only org members enabled)",
			onlyOrgMembers:  true,
			user:            "test-collaborator",
			org:             "kubernetes",
			repo:            "kubernetes",
			expectedTrusted: false,
			expectedReason:  (notMember).String(),
		},
		{
			name:            "user is trusted org member",
			onlyOrgMembers:  false,
			trustedOrg:      "kubernetes",
			user:            "test",
			org:             "kubernetes-sigs",
			repo:            "test",
			expectedTrusted: true,
		},
		{
			name:            "user is not org member",
			onlyOrgMembers:  false,
			user:            "test-2",
			org:             "kubernetes",
			repo:            "kubernetes",
			expectedTrusted: false,
			expectedReason:  (notMember | notCollaborator).String(),
		},
		{
			name:            "user is not org member or trusted org member",
			onlyOrgMembers:  false,
			trustedOrg:      "kubernetes-sigs",
			user:            "test-2",
			org:             "kubernetes",
			repo:            "kubernetes",
			expectedTrusted: false,
			expectedReason:  (notMember | notCollaborator | notSecondaryMember).String(),
		},
		{
			name:            "user is not org member or trusted org member, onlyOrgMembers true",
			onlyOrgMembers:  true,
			trustedOrg:      "kubernetes-sigs",
			user:            "test-2",
			org:             "kubernetes",
			repo:            "kubernetes",
			expectedTrusted: false,
			expectedReason:  (notMember | notSecondaryMember).String(),
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fc := &fakegithub.FakeClient{
				OrgMembers: map[string][]string{
					"kubernetes": {"test"},
				},
				Collaborators: []string{"test-collaborator"},
			}

			trustedResponse, err := TrustedUser(fc, tc.onlyOrgMembers, tc.trustedOrg, tc.user, tc.org, tc.repo)
			if err != nil {
				t.Errorf("For case %s, didn't expect error from TrustedUser: %v", tc.name, err)
			}
			if trustedResponse.IsTrusted != tc.expectedTrusted {
				t.Errorf("For case %s, expect trusted: %v, but got: %v", tc.name, tc.expectedTrusted, trustedResponse.IsTrusted)
			}
			if trustedResponse.Reason != tc.expectedReason {
				t.Errorf("For case %s, expect trusted reason: %v, but got: %v", tc.name, tc.expectedReason, trustedResponse.Reason)
			}
		})
	}
}

func TestGetPresubmits(t *testing.T) {
	const orgRepo = "my-org/my-repo"

	testCases := []struct {
		name string
		cfg  *config.Config

		expectedPresubmits sets.String
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
					ProwYAMLGetter: func(_ *config.Config, _ git.ClientFactory, _, _ string, _ ...string) (*config.ProwYAML, error) {
						return &config.ProwYAML{
							Presubmits: []config.Presubmit{{
								JobBase: config.JobBase{Name: "my-inrepoconfig-presubmit"},
							}},
						}, nil
					},
				},
				ProwConfig: config.ProwConfig{
					InRepoConfig: config.InRepoConfig{Enabled: map[string]*bool{"*": utilpointer.BoolPtr(true)}},
				},
			},

			expectedPresubmits: sets.NewString("my-inrepoconfig-presubmit", "my-static-presubmit"),
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
					ProwYAMLGetter: func(_ *config.Config, _ git.ClientFactory, _, _ string, _ ...string) (*config.ProwYAML, error) {
						return &config.ProwYAML{
							Presubmits: []config.Presubmit{{
								JobBase: config.JobBase{Name: "my-inrepoconfig-presubmit"},
							}},
						}, errors.New("some error")
					},
				},
				ProwConfig: config.ProwConfig{
					InRepoConfig: config.InRepoConfig{Enabled: map[string]*bool{"*": utilpointer.BoolPtr(true)}},
				},
			},

			expectedPresubmits: sets.NewString("my-static-presubmit"),
		},
	}

	shaGetter := func() (string, error) {
		return "", nil
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			presubmits := getPresubmits(logrus.NewEntry(logrus.New()), nil, tc.cfg, orgRepo, shaGetter, shaGetter)
			actualPresubmits := sets.String{}
			for _, presubmit := range presubmits {
				actualPresubmits.Insert(presubmit.Name)
			}

			if !tc.expectedPresubmits.Equal(actualPresubmits) {
				t.Errorf("got a different set of presubmits than expected, diff: %v", tc.expectedPresubmits.Difference(actualPresubmits))
			}
		})
	}
}

func TestGetPostsubmits(t *testing.T) {
	const orgRepo = "my-org/my-repo"

	testCases := []struct {
		name string
		cfg  *config.Config

		expectedPostsubmits sets.String
	}{
		{
			name: "Result of GetPostsubmits is used by default",
			cfg: &config.Config{
				JobConfig: config.JobConfig{
					PostsubmitsStatic: map[string][]config.Postsubmit{
						orgRepo: {{
							JobBase: config.JobBase{Name: "my-static-postsubmit"},
						}},
					},
					ProwYAMLGetter: func(_ *config.Config, _ git.ClientFactory, _, _ string, _ ...string) (*config.ProwYAML, error) {
						return &config.ProwYAML{
							Postsubmits: []config.Postsubmit{{
								JobBase: config.JobBase{Name: "my-inrepoconfig-postsubmit"},
							}},
						}, nil
					},
				},
				ProwConfig: config.ProwConfig{
					InRepoConfig: config.InRepoConfig{Enabled: map[string]*bool{"*": utilpointer.BoolPtr(true)}},
				},
			},

			expectedPostsubmits: sets.NewString("my-inrepoconfig-postsubmit", "my-static-postsubmit"),
		},
		{
			name: "Fallback to static postsubmits",
			cfg: &config.Config{
				JobConfig: config.JobConfig{
					PostsubmitsStatic: map[string][]config.Postsubmit{
						orgRepo: {{
							JobBase: config.JobBase{Name: "my-static-postsubmit"},
						}},
					},
					ProwYAMLGetter: func(_ *config.Config, _ git.ClientFactory, _, _ string, _ ...string) (*config.ProwYAML, error) {
						return &config.ProwYAML{
							Postsubmits: []config.Postsubmit{{
								JobBase: config.JobBase{Name: "my-inrepoconfig-postsubmit"},
							}},
						}, errors.New("some error")
					},
				},
				ProwConfig: config.ProwConfig{
					InRepoConfig: config.InRepoConfig{Enabled: map[string]*bool{"*": utilpointer.BoolPtr(true)}},
				},
			},

			expectedPostsubmits: sets.NewString("my-static-postsubmit"),
		},
	}

	shaGetter := func() (string, error) {
		return "", nil
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			postsubmits := getPostsubmits(logrus.NewEntry(logrus.New()), nil, tc.cfg, orgRepo, shaGetter)
			actualPostsubmits := sets.String{}
			for _, postsubmit := range postsubmits {
				actualPostsubmits.Insert(postsubmit.Name)
			}

			if !tc.expectedPostsubmits.Equal(actualPostsubmits) {
				t.Errorf("got a different set of postsubmits than expected, diff: %v", tc.expectedPostsubmits.Difference(actualPostsubmits))
			}
		})
	}
}

func TestCreateWithRetry(t *testing.T) {
	testCases := []struct {
		name            string
		numFailedCreate int
		expectedErrMsg  string
	}{
		{
			name: "Initial success",
		},
		{
			name:            "Success after retry",
			numFailedCreate: 7,
		},
		{
			name:            "Failure",
			numFailedCreate: 8,
			expectedErrMsg:  "need retrying",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {

			fakeProwJobClient := fake.NewSimpleClientset()
			fakeProwJobClient.PrependReactor("*", "*", func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
				if _, ok := action.(clienttesting.CreateActionImpl); ok && tc.numFailedCreate > 0 {
					tc.numFailedCreate--
					return true, nil, errors.New("need retrying")
				}
				return false, nil, nil
			})

			pj := &prowapi.ProwJob{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}

			var errMsg string
			err := createWithRetry(context.TODO(), fakeProwJobClient.ProwV1().ProwJobs(""), pj, time.Nanosecond)
			if err != nil {
				errMsg = err.Error()
			}
			if errMsg != tc.expectedErrMsg {
				t.Fatalf("expected error %s, got error %v", tc.expectedErrMsg, err)
			}
			if err != nil {
				return
			}

			result, err := fakeProwJobClient.ProwV1().ProwJobs("").List(context.Background(), metav1.ListOptions{})
			if err != nil {
				t.Fatalf("faile to list prowjobs: %v", err)
			}

			if len(result.Items) != 1 {
				t.Errorf("expected to find exactly one prowjob, got %+v", result.Items)
			}
		})
	}
}
