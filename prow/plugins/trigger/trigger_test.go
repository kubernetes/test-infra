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
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	clienttesting "k8s.io/client-go/testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/client/clientset/versioned/fake"
	"k8s.io/test-infra/prow/config"
	fakegit "k8s.io/test-infra/prow/git/localgit"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	inrepoconfigapi "k8s.io/test-infra/prow/inrepoconfig/api"
	"k8s.io/test-infra/prow/plugins"
)

func TestHelpProvider(t *testing.T) {
	cases := []struct {
		name         string
		config       *plugins.Configuration
		enabledRepos []string
		err          bool
	}{
		{
			name:         "Empty config",
			config:       &plugins.Configuration{},
			enabledRepos: []string{"org1", "org2/repo"},
		},
		{
			name:         "Overlapping org and org/repo",
			config:       &plugins.Configuration{},
			enabledRepos: []string{"org2", "org2/repo"},
		},
		{
			name:         "Invalid enabledRepos",
			config:       &plugins.Configuration{},
			enabledRepos: []string{"org1", "org2/repo/extra"},
			err:          true,
		},
		{
			name: "All configs enabled",
			config: &plugins.Configuration{
				Triggers: []plugins.Trigger{
					{
						Repos:          []string{"org2"},
						TrustedOrg:     "org2",
						JoinOrgURL:     "https://join.me",
						OnlyOrgMembers: true,
						IgnoreOkToTest: true,
					},
				},
			},
			enabledRepos: []string{"org1", "org2/repo"},
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

func TestRunAndSkipJobs(t *testing.T) {
	var testCases = []struct {
		name string

		requestedJobs        []config.Presubmit
		skippedJobs          []config.Presubmit
		elideSkippedContexts bool
		jobCreationErrs      sets.String // job names which fail creation

		expectedJobs     sets.String // by name
		expectedStatuses []github.Status
		expectedErr      bool
	}{
		{
			name: "nothing requested means nothing done",
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
		{
			name: "all skipped jobs get skipped",
			skippedJobs: []config.Presubmit{{
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
			expectedStatuses: []github.Status{{
				State:       github.StatusSuccess,
				Context:     "first-context",
				Description: "Skipped.",
			}, {
				State:       github.StatusSuccess,
				Context:     "second-context",
				Description: "Skipped.",
			}},
		},
		{
			name: "all skipped jobs get ignored if skipped statuses are elided",
			skippedJobs: []config.Presubmit{{
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
			elideSkippedContexts: true,
		},
		{
			name: "skipped jobs with skip report get ignored",
			skippedJobs: []config.Presubmit{{
				JobBase: config.JobBase{
					Name: "first",
				},
				Reporter: config.Reporter{Context: "first-context"},
			}, {
				JobBase: config.JobBase{
					Name: "second",
				},
				Reporter: config.Reporter{Context: "second-context", SkipReport: true},
			}},
			expectedStatuses: []github.Status{{
				State:       github.StatusSuccess,
				Context:     "first-context",
				Description: "Skipped.",
			}},
		},
		{
			name: "overlap between jobs callErrors and has no external action",
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
			skippedJobs: []config.Presubmit{{
				JobBase: config.JobBase{
					Name: "first",
				},
				Reporter: config.Reporter{Context: "first-context"},
			}},
			expectedErr: true,
		},
		{
			name: "disjoint sets of jobs get triggered and skipped correctly",
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
			skippedJobs: []config.Presubmit{{
				JobBase: config.JobBase{
					Name: "third",
				},
				Reporter: config.Reporter{Context: "third-context"},
			}, {
				JobBase: config.JobBase{
					Name: "fourth",
				},
				Reporter: config.Reporter{Context: "fourth-context"},
			}},
			expectedJobs: sets.NewString("first", "second"),
			expectedStatuses: []github.Status{{
				State:       github.StatusSuccess,
				Context:     "third-context",
				Description: "Skipped.",
			}, {
				State:       github.StatusSuccess,
				Context:     "fourth-context",
				Description: "Skipped.",
			}},
		},
		{
			name: "disjoint sets of jobs get triggered and skipped correctly, even if one creation fails",
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
			skippedJobs: []config.Presubmit{{
				JobBase: config.JobBase{
					Name: "third",
				},
				Reporter: config.Reporter{Context: "third-context"},
			}, {
				JobBase: config.JobBase{
					Name: "fourth",
				},
				Reporter: config.Reporter{Context: "fourth-context"},
			}},
			jobCreationErrs: sets.NewString("first"),
			expectedJobs:    sets.NewString("second"),
			expectedStatuses: []github.Status{{
				State:       github.StatusSuccess,
				Context:     "third-context",
				Description: "Skipped.",
			}, {
				State:       github.StatusSuccess,
				Context:     "fourth-context",
				Description: "Skipped.",
			}},
			expectedErr: true,
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
		fakeGitHubClient := fakegithub.FakeClient{}
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

		err := RunAndSkipJobs(client, pr, testCase.requestedJobs, testCase.skippedJobs, "", "event-guid", testCase.elideSkippedContexts)
		if err == nil && testCase.expectedErr {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}
		if err != nil && !testCase.expectedErr {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
		}

		if actual, expected := fakeGitHubClient.CreatedStatuses[pr.Head.SHA], testCase.expectedStatuses; !reflect.DeepEqual(actual, expected) {
			t.Errorf("%s: created incorrect statuses: %s", testCase.name, diff.ObjectReflectDiff(actual, expected))
		}

		observedCreatedProwJobs := sets.NewString()
		existingProwJobs, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").List(metav1.ListOptions{})
		if err != nil {
			t.Errorf("%s: could not list current state of prow jobs: %v", testCase.name, err)
			continue
		}
		for _, job := range existingProwJobs.Items {
			observedCreatedProwJobs.Insert(job.Spec.Job)
		}

		if missing := testCase.expectedJobs.Difference(observedCreatedProwJobs); missing.Len() > 0 {
			t.Errorf("%s: didn't create all expected ProwJobs, missing: %s", testCase.name, missing.List())
		}
		if extra := observedCreatedProwJobs.Difference(testCase.expectedJobs); extra.Len() > 0 {
			t.Errorf("%s: created unexpected ProwJobs: %s", testCase.name, extra.List())
		}
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
		fakeGitHubClient := fakegithub.FakeClient{}
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

		err := runRequested(client, pr, testCase.requestedJobs, "", "event-guid")
		if err == nil && testCase.expectedErr {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}
		if err != nil && !testCase.expectedErr {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
		}

		observedCreatedProwJobs := sets.NewString()
		existingProwJobs, err := fakeProwJobClient.ProwV1().ProwJobs("prowjobs").List(metav1.ListOptions{})
		if err != nil {
			t.Errorf("%s: could not list current state of prow jobs: %v", testCase.name, err)
			continue
		}
		for _, job := range existingProwJobs.Items {
			observedCreatedProwJobs.Insert(job.Spec.Job)
		}

		if missing := testCase.expectedJobs.Difference(observedCreatedProwJobs); missing.Len() > 0 {
			t.Errorf("%s: didn't create all expected ProwJobs, missing: %s", testCase.name, missing.List())
		}
		if extra := observedCreatedProwJobs.Difference(testCase.expectedJobs); extra.Len() > 0 {
			t.Errorf("%s: created unexpected ProwJobs: %s", testCase.name, extra.List())
		}
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

func TestRunAndSkipJobsFetchesBaseSHAOnlyIfEmpty(t *testing.T) {
	tcs := []struct {
		name            string
		baseSHA         string
		expectedBaseSHA string
	}{
		{
			name:            "Passed in BaseSHA is used",
			baseSHA:         "abc",
			expectedBaseSHA: "abc",
		},
		{
			name:            "BaseSHA is empty and fetched from GitHub",
			expectedBaseSHA: fakegithub.TestRef,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			fakeGitHubClient := fakegithub.FakeClient{}
			fakeProwJobClient := fake.NewSimpleClientset()
			client := Client{
				GitHubClient:  &fakeGitHubClient,
				ProwJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
				Logger:        logrus.WithField("testcase", tc.name),
			}
			requestedJobs := []config.Presubmit{{
				JobBase: config.JobBase{
					Name: "first",
				}}}
			if err := RunAndSkipJobs(
				client, &github.PullRequest{}, requestedJobs, nil, tc.baseSHA, "", false); err != nil {
				t.Fatalf("executing RunAndSkipJobs failed: %v", err)
			}

			pjs, err := fakeProwJobClient.ProwV1().ProwJobs("").List(metav1.ListOptions{})
			if err != nil {
				t.Fatalf("failed to list prowjobs: %v", err)
			}
			if len(pjs.Items) != 1 {
				t.Fatalf("expected exactly one prowjob, got %d", len(pjs.Items))
			}

			if pjs.Items[0].Spec.Refs.BaseSHA != tc.expectedBaseSHA {
				t.Errorf("expected baseSHA to be %q, was %q", tc.expectedBaseSHA, pjs.Items[0].Spec.Refs.BaseSHA)
			}
		})
	}
}

func TestGetPresubmitsForPR(t *testing.T) {
	baseRepoOwner := "test-org"
	baseRepoName := "test-repo"
	baseRepoFullName := baseRepoOwner + "/" + baseRepoName

	tcs := []struct {
		name                      string
		presubmitsFromConfig      []config.Presubmit
		presubmitsInRepo          []config.Presubmit
		inRepoConfigEnabled       bool
		expectedPresubmits        sets.String
		gitHubInteractionExpected bool
	}{
		{
			name:                 "InRepoConfig disabled, only presubmits from config",
			presubmitsFromConfig: []config.Presubmit{{JobBase: config.JobBase{Name: "my-presubmit"}}},
			expectedPresubmits:   sets.NewString("my-presubmit"),
		},
		{
			name:                      "InRepoConfig enabled, only config has presubmits",
			presubmitsFromConfig:      []config.Presubmit{{JobBase: config.JobBase{Name: "my-presubmit"}}},
			inRepoConfigEnabled:       true,
			expectedPresubmits:        sets.NewString("my-presubmit"),
			gitHubInteractionExpected: true,
		},
		{
			name: "InRepoConfig enabled, only prow.yaml has presubmits",
			presubmitsInRepo: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "my-ir-presubmit",
						Spec: &corev1.PodSpec{Containers: []corev1.Container{{}}},
					},
				},
			},
			inRepoConfigEnabled:       true,
			expectedPresubmits:        sets.NewString("my-ir-presubmit"),
			gitHubInteractionExpected: true,
		},
		{
			name:                 "InRepoConfig enabled, combined presubmits from config and repo",
			presubmitsFromConfig: []config.Presubmit{{JobBase: config.JobBase{Name: "my-presubmit"}}},
			presubmitsInRepo: []config.Presubmit{
				{
					JobBase: config.JobBase{
						Name: "my-ir-presubmit",
						Spec: &corev1.PodSpec{Containers: []corev1.Container{{}}},
					},
				},
			},
			inRepoConfigEnabled:       true,
			expectedPresubmits:        sets.NewString("my-presubmit", "my-ir-presubmit"),
			gitHubInteractionExpected: true,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			fg, gitClient, err := fakegit.New()
			if err != nil {
				t.Fatalf("Making local git repo: %v", err)
			}
			defer func() {
				if err := fg.Clean(); err != nil {
					t.Errorf("Error cleaning LocalGit: %v", err)
				}
				if err := gitClient.Clean(); err != nil {
					t.Errorf("Error cleaning Client: %v", err)
				}
			}()

			fakeGitHubClient := fakegithub.FakeClient{}
			fakeProwJobClient := fake.NewSimpleClientset()
			client := Client{
				Config: &config.Config{
					JobConfig: config.JobConfig{
						Presubmits: map[string][]config.Presubmit{
							baseRepoFullName: tc.presubmitsFromConfig,
						},
					},
					ProwConfig: config.ProwConfig{
						PodNamespace: "my-pod-ns",
					},
				},
				GitHubClient:  &fakeGitHubClient,
				ProwJobClient: fakeProwJobClient.ProwV1().ProwJobs("prowjobs"),
				Logger:        logrus.WithField("testcase", tc.name),
				GitClient:     gitClient,
			}
			pr := &github.PullRequest{
				Base: github.PullRequestBranch{
					Repo: github.Repo{
						Owner: github.User{
							Login: baseRepoOwner,
						},
						Name:     baseRepoName,
						FullName: baseRepoFullName},
				},
			}
			for i := range tc.presubmitsFromConfig {
				config.DefaultPresubmitFields(&client.Config.ProwConfig, &tc.presubmitsFromConfig[i])
				tc.presubmitsFromConfig[i].Spec = &corev1.PodSpec{
					Containers: []corev1.Container{{}},
				}
			}

			if tc.inRepoConfigEnabled {
				client.Config.InRepoConfig = map[string]config.InRepoConfig{
					"*": {Enabled: true}}
				inRepoConfig := inrepoconfigapi.InRepoConfig{
					Presubmits: tc.presubmitsInRepo,
				}
				marshalledInRepoConfig, err := json.Marshal(inRepoConfig)
				if err != nil {
					t.Fatalf("failed to marshal inrepoconfig: %v", err)
				}
				if err := fg.MakeFakeRepo(baseRepoOwner, baseRepoName); err != nil {
					t.Fatalf("Making fake repo: %v", err)
				}
				if err := fg.CheckoutNewBranch(baseRepoOwner, baseRepoName, "my-pull"); err != nil {
					t.Fatalf("failed to check out new branch: %v", err)
				}
				if err := fg.AddCommit(baseRepoOwner, baseRepoName, map[string][]byte{
					inrepoconfigapi.ConfigFileName: marshalledInRepoConfig,
				}); err != nil {
					t.Fatalf("Failed to add commit: %v", err)
				}

				masterHeadHash, err := fg.RevParse(baseRepoOwner, baseRepoName, "master")
				if err != nil {
					t.Fatalf("failed to run git rev-parse master: %v", err)
				}
				fakegithub.TestRef = masterHeadHash
				pr.Base.Ref = masterHeadHash

				pr.Head.SHA, err = fg.RevParse(baseRepoOwner, baseRepoName, "HEAD")
				if err != nil {
					t.Fatalf("failed to run git rev-parse HEAD: %v", err)
				}
			}

			_, presubmits, err := getPresubmitsForPR(client, pr)
			if err != nil {
				t.Fatalf("failed to call getPresubmitsForPR: %v", err)
			}

			actualPresumits := sets.NewString()
			for _, presubmit := range presubmits {
				actualPresumits.Insert(presubmit.Name)
			}
			if diff := actualPresumits.Difference(tc.expectedPresubmits).List(); len(diff) > 0 {
				t.Errorf("actual presubmits did not match expected presubmits, diff: %v", diff)
			}

			hasGitHubInteraction := fakeGitHubClient.CreatedStatuses != nil
			if tc.gitHubInteractionExpected != hasGitHubInteraction {
				t.Errorf("Expected GitHub interaction: %t, had GitHub interaction: %t",
					tc.gitHubInteractionExpected, hasGitHubInteraction)
			}

		})
	}
}
