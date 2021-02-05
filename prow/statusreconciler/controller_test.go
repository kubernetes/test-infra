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

package statusreconciler

import (
	"errors"
	"reflect"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
)

func TestAddedBlockingPresubmits(t *testing.T) {
	var testCases = []struct {
		name     string
		old, new string
		expected map[string][]config.Presubmit
	}{
		{
			name: "no change in blocking presubmits means no added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  always_run: true`,
			new: `"org/repo":
- name: old-job
  context: old-context
  always_run: true`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "added optional presubmit means no added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  always_run: true`,
			new: `"org/repo":
- name: old-job
  context: old-context
  always_run: true
- name: new-job
  context: new-context
  always_run: true
  optional: true`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "added non-reporting presubmit means no added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  always_run: true`,
			new: `"org/repo":
- name: old-job
  context: old-context
  always_run: true
- name: new-job
  context: new-context
  always_run: true
  skip_report: true`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "added presubmit that needs a manual trigger means no added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  always_run: true`,
			new: `"org/repo":
- name: old-job
  context: old-context
  always_run: true
- name: new-job
  context: new-context
  always_run: false`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "added required presubmit means added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  always_run: true`,
			new: `"org/repo":
- name: old-job
  context: old-context
  always_run: true
- name: new-job
  context: new-context
  always_run: true`,
			expected: map[string][]config.Presubmit{
				"org/repo": {{
					JobBase: config.JobBase{Name: "new-job"},
					Reporter: config.Reporter{
						Context:    "new-context",
						SkipReport: false,
					},
					AlwaysRun: true,
					Optional:  false,
				}},
			},
		},
		{
			name: "optional presubmit transitioning to required means no added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  always_run: true
  optional: true`,
			new: `"org/repo":
- name: old-job
  context: old-context
  always_run: true`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "non-reporting presubmit transitioning to required means added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  always_run: true
  skip_report: true`,
			new: `"org/repo":
- name: old-job
  context: old-context
  always_run: true`,
			expected: map[string][]config.Presubmit{
				"org/repo": {{
					JobBase:   config.JobBase{Name: "old-job"},
					Reporter:  config.Reporter{Context: "old-context"},
					AlwaysRun: true,
				}},
			},
		},
		{
			name: "required presubmit transitioning run_if_changed means added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  run_if_changed: old-changes`,
			new: `"org/repo":
- name: old-job
  context: old-context
  run_if_changed: new-changes`,
			expected: map[string][]config.Presubmit{
				"org/repo": {{
					JobBase:             config.JobBase{Name: "old-job"},
					Reporter:            config.Reporter{Context: "old-context"},
					RegexpChangeMatcher: config.RegexpChangeMatcher{RunIfChanged: "new-changes"},
				}},
			},
		},
		{
			name: "optional presubmit transitioning run_if_changed means no added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  run_if_changed: old-changes
  optional: true`,
			new: `"org/repo":
- name: old-job
  context: old-context
  run_if_changed: new-changes
  optional: true`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "optional presubmit transitioning to required run_if_changed means added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  always_run: true
  optional: true`,
			new: `"org/repo":
- name: old-job
  context: old-context
  run_if_changed: changes`,
			expected: map[string][]config.Presubmit{
				"org/repo": {{
					JobBase:             config.JobBase{Name: "old-job"},
					Reporter:            config.Reporter{Context: "old-context"},
					RegexpChangeMatcher: config.RegexpChangeMatcher{RunIfChanged: "changes"},
				}},
			},
		},
		{
			name: "required presubmit transitioning to new context means no added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  always_run: true`,
			new: `"org/repo":
- name: old-job
  context: new-context
  always_run: true`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var oldConfig, newConfig map[string][]config.Presubmit
			if err := yaml.Unmarshal([]byte(testCase.old), &oldConfig); err != nil {
				t.Fatalf("%s: could not unmarshal old config: %v", testCase.name, err)
			}
			if err := yaml.Unmarshal([]byte(testCase.new), &newConfig); err != nil {
				t.Fatalf("%s: could not unmarshal new config: %v", testCase.name, err)
			}
			if actual, _ := addedBlockingPresubmits(oldConfig, newConfig, logrusEntry()); !reflect.DeepEqual(actual, testCase.expected) {
				t.Errorf("%s: did not get correct added presubmits: %v", testCase.name, diff.ObjectReflectDiff(actual, testCase.expected))
			}
		})
	}
}

func TestRemovedBlockingPresubmits(t *testing.T) {
	var testCases = []struct {
		name     string
		old, new string
		expected map[string][]config.Presubmit
	}{
		{
			name: "no change in blocking presubmits means no removed blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `"org/repo":
- name: old-job
  context: old-context`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "removed optional presubmit means no removed blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  optional: true`,
			new: `"org/repo": []`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "removed non-reporting presubmit means no removed blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  skip_report: true`,
			new: `"org/repo": []`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "removed required presubmit means removed blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `"org/repo": []`,
			expected: map[string][]config.Presubmit{
				"org/repo": {{
					JobBase:  config.JobBase{Name: "old-job"},
					Reporter: config.Reporter{Context: "old-context"},
				}},
			},
		},
		{
			name: "required presubmit transitioning to optional means no removed blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `"org/repo":
- name: old-job
  context: old-context
  optional: true`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "reporting presubmit transitioning to non-reporting means no removed blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `"org/repo":
- name: old-job
  context: old-context
  skip_report: true`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "all presubmits removed means removed blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `{}`,
			expected: map[string][]config.Presubmit{
				"org/repo": {{
					JobBase:  config.JobBase{Name: "old-job"},
					Reporter: config.Reporter{Context: "old-context"},
				}},
			},
		},
		{
			name: "required presubmit transitioning to new context means no removed blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `"org/repo":
- name: old-job
  context: new-context`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "required presubmit transitioning run_if_changed means no removed blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  run_if_changed: old-changes`,
			new: `"org/repo":
- name: old-job
  context: old-context
  run_if_changed: new-changes`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "optional presubmit transitioning to required run_if_changed means no removed blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  optional: true`,
			new: `"org/repo":
- name: old-job
  context: old-context
  run_if_changed: changes`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var oldConfig, newConfig map[string][]config.Presubmit
			if err := yaml.Unmarshal([]byte(testCase.old), &oldConfig); err != nil {
				t.Fatalf("%s: could not unmarshal old config: %v", testCase.name, err)
			}
			if err := yaml.Unmarshal([]byte(testCase.new), &newConfig); err != nil {
				t.Fatalf("%s: could not unmarshal new config: %v", testCase.name, err)
			}
			if actual, _ := removedBlockingPresubmits(oldConfig, newConfig, logrusEntry()); !reflect.DeepEqual(actual, testCase.expected) {
				t.Errorf("%s: did not get correct removed presubmits: %v", testCase.name, diff.ObjectReflectDiff(actual, testCase.expected))
			}
		})
	}
}

func TestMigratedBlockingPresubmits(t *testing.T) {
	var testCases = []struct {
		name     string
		old, new string
		expected map[string][]presubmitMigration
	}{
		{
			name: "no change in blocking presubmits means no migrated blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `"org/repo":
- name: old-job
  context: old-context`,
			expected: map[string][]presubmitMigration{
				"org/repo": {},
			},
		},
		{
			name: "removed optional presubmit means no migrated blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  optional: true`,
			new: `"org/repo": []`,
			expected: map[string][]presubmitMigration{
				"org/repo": {},
			},
		},
		{
			name: "removed non-reporting presubmit means no migrated blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  skip_report: true`,
			new: `"org/repo": []`,
			expected: map[string][]presubmitMigration{
				"org/repo": {},
			},
		},
		{
			name: "removed required presubmit means no migrated blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `"org/repo": []`,
			expected: map[string][]presubmitMigration{
				"org/repo": {},
			},
		},
		{
			name: "required presubmit transitioning to optional means no migrated blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `"org/repo":
- name: old-job
  context: old-context
  optional: true`,
			expected: map[string][]presubmitMigration{
				"org/repo": {},
			},
		},
		{
			name: "reporting presubmit transitioning to non-reporting means no migrated blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `"org/repo":
- name: old-job
  context: old-context
  skip_report: true`,
			expected: map[string][]presubmitMigration{
				"org/repo": {},
			},
		},
		{
			name: "all presubmits removed means no migrated blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `{}`,
			expected: map[string][]presubmitMigration{
				"org/repo": {},
			},
		},
		{
			name: "required presubmit transitioning to new context means migrated blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `"org/repo":
- name: old-job
  context: new-context`,
			expected: map[string][]presubmitMigration{
				"org/repo": {{
					from: config.Presubmit{
						JobBase:  config.JobBase{Name: "old-job"},
						Reporter: config.Reporter{Context: "old-context"},
					},
					to: config.Presubmit{
						JobBase:  config.JobBase{Name: "old-job"},
						Reporter: config.Reporter{Context: "new-context"},
					},
				}},
			},
		},
		{
			name: "required presubmit transitioning run_if_changed means no removed blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  run_if_changed: old-changes`,
			new: `"org/repo":
- name: old-job
  context: old-context
  run_if_changed: new-changes`,
			expected: map[string][]presubmitMigration{
				"org/repo": {},
			},
		},
		{
			name: "optional presubmit transitioning to required run_if_changed means no removed blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  optional: true`,
			new: `"org/repo":
- name: old-job
  context: old-context
  run_if_changed: changes`,
			expected: map[string][]presubmitMigration{
				"org/repo": {},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			var oldConfig, newConfig map[string][]config.Presubmit
			if err := yaml.Unmarshal([]byte(testCase.old), &oldConfig); err != nil {
				t.Fatalf("%s: could not unmarshal old config: %v", testCase.name, err)
			}
			if err := yaml.Unmarshal([]byte(testCase.new), &newConfig); err != nil {
				t.Fatalf("%s: could not unmarshal new config: %v", testCase.name, err)
			}
			if actual, _ := migratedBlockingPresubmits(oldConfig, newConfig, logrusEntry()); !reflect.DeepEqual(actual, testCase.expected) {
				t.Errorf("%s: did not get correct removed presubmits: %v", testCase.name, diff.ObjectReflectDiff(actual, testCase.expected))
			}
		})
	}
}

type orgRepo struct {
	org, repo string
}

type orgRepoSet map[orgRepo]interface{}

func (s orgRepoSet) has(item orgRepo) bool {
	_, contained := s[item]
	return contained
}

type migration struct {
	from, to string
}

type migrationSet map[migration]interface{}

func (s migrationSet) insert(items ...migration) {
	for _, item := range items {
		s[item] = nil
	}
}

func (s migrationSet) has(item migration) bool {
	_, contained := s[item]
	return contained
}

func newFakeMigrator(key orgRepo) fakeMigrator {
	return fakeMigrator{
		retireErrors:  map[orgRepo]sets.String{key: sets.NewString()},
		migrateErrors: map[orgRepo]migrationSet{key: {}},
		retired:       map[orgRepo]sets.String{key: sets.NewString()},
		migrated:      map[orgRepo]migrationSet{key: {}},
	}
}

type fakeMigrator struct {
	retireErrors  map[orgRepo]sets.String
	migrateErrors map[orgRepo]migrationSet

	retired  map[orgRepo]sets.String
	migrated map[orgRepo]migrationSet
}

func (m *fakeMigrator) retire(org, repo, context string, targetBranchFilter func(string) bool) error {
	key := orgRepo{org: org, repo: repo}
	if contexts, exist := m.retireErrors[key]; exist && contexts.Has(context) {
		return errors.New("failed to retire context")
	}
	if _, exist := m.retired[key]; exist {
		m.retired[key].Insert(context)
	} else {
		m.retired[key] = sets.NewString(context)
	}
	return nil
}

func (m *fakeMigrator) migrate(org, repo, from, to string, targetBranchFilter func(string) bool) error {
	key := orgRepo{org: org, repo: repo}
	item := migration{from: from, to: to}
	if contexts, exist := m.migrateErrors[key]; exist && contexts.has(item) {
		return errors.New("failed to migrate context")
	}
	if _, exist := m.migrated[key]; exist {
		m.migrated[key].insert(item)
	} else {
		newSet := migrationSet{}
		newSet.insert(item)
		m.migrated[key] = newSet
	}
	return nil
}

func newfakeProwJobTriggerer() fakeProwJobTriggerer {
	return fakeProwJobTriggerer{
		errors:  map[prKey]sets.String{},
		created: map[prKey]sets.String{},
	}
}

type prKey struct {
	org, repo string
	num       int
}

type fakeProwJobTriggerer struct {
	errors  map[prKey]sets.String
	created map[prKey]sets.String
}

func (c *fakeProwJobTriggerer) runAndSkip(pr *github.PullRequest, requestedJobs []config.Presubmit) error {
	actions := []struct {
		jobs    []config.Presubmit
		records map[prKey]sets.String
	}{
		{
			jobs:    requestedJobs,
			records: c.created,
		},
	}
	for _, action := range actions {
		names := sets.NewString()
		key := prKey{org: pr.Base.Repo.Owner.Login, repo: pr.Base.Repo.Name, num: pr.Number}
		for _, job := range action.jobs {
			if jobErrors, exists := c.errors[key]; exists && jobErrors.Has(job.Name) {
				return errors.New("failed to trigger prow job")
			}
			names.Insert(job.Name)
		}
		if current, exists := action.records[key]; exists {
			action.records[key] = current.Union(names)
		} else {
			action.records[key] = names
		}
	}
	return nil
}

func newFakeGitHubClient(key orgRepo) fakeGitHubClient {
	return fakeGitHubClient{
		prErrors:  orgRepoSet{},
		refErrors: map[orgRepo]sets.String{key: sets.NewString()},
		prs:       map[orgRepo][]github.PullRequest{key: {}},
		refs:      map[orgRepo]map[string]string{key: {}},
	}
}

type fakeGitHubClient struct {
	prErrors     orgRepoSet
	refErrors    map[orgRepo]sets.String
	changeErrors map[orgRepo]sets.Int

	prs     map[orgRepo][]github.PullRequest
	refs    map[orgRepo]map[string]string
	changes map[orgRepo]map[int][]github.PullRequestChange
}

func (c *fakeGitHubClient) GetPullRequests(org, repo string) ([]github.PullRequest, error) {
	key := orgRepo{org: org, repo: repo}
	if c.prErrors.has(key) {
		return nil, errors.New("failed to get PRs")
	}
	return c.prs[key], nil
}

func (c *fakeGitHubClient) GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error) {
	key := orgRepo{org: org, repo: repo}
	if changes, exist := c.changeErrors[key]; exist && changes.Has(number) {
		return nil, errors.New("failed to get changes")
	}
	return c.changes[key][number], nil
}

func (c *fakeGitHubClient) GetRef(org, repo, ref string) (string, error) {
	key := orgRepo{org: org, repo: repo}
	if refs, exist := c.refErrors[key]; exist && refs.Has(ref) {
		return "", errors.New("failed to get ref")
	}
	return c.refs[key][ref], nil
}

type prAuthor struct {
	pr     int
	author string
}

type prAuthorSet map[prAuthor]interface{}

func (s prAuthorSet) has(item prAuthor) bool {
	_, contained := s[item]
	return contained
}

func newFakeTrustedChecker(key orgRepo) fakeTrustedChecker {
	return fakeTrustedChecker{
		errors:  map[orgRepo]prAuthorSet{key: {}},
		trusted: map[orgRepo]map[prAuthor]bool{key: {}},
	}
}

type fakeTrustedChecker struct {
	errors map[orgRepo]prAuthorSet

	trusted map[orgRepo]map[prAuthor]bool
}

func (c *fakeTrustedChecker) trustedPullRequest(author, org, repo string, num int) (bool, error) {
	key := orgRepo{org: org, repo: repo}
	item := prAuthor{pr: num, author: author}
	if errs, exist := c.errors[key]; exist && errs.has(item) {
		return false, errors.New("failed to check trusted")
	}
	return c.trusted[key][item], nil
}

func TestControllerReconcile(t *testing.T) {
	// the diff from these configs causes:
	//  - deletion (required-job),
	//  - creation (new-required-job)
	//  - migration (other-required-job)
	oldConfigData := `presubmits:
  "org/repo":
  - name: required-job
    context: required-job
    always_run: true
  - name: other-required-job
    context: other-required-job
    always_run: true`
	newConfigData := `presubmits:
  "org/repo":
  - name: other-required-job
    context: new-context
    always_run: true
  - name: new-required-job
    context: new-required-context
    always_run: true
    branches:
    - base`

	var oldConfig, newConfig config.Config
	if err := yaml.Unmarshal([]byte(oldConfigData), &oldConfig); err != nil {
		t.Fatalf("could not unmarshal old config: %v", err)
	}
	for _, presubmits := range oldConfig.PresubmitsStatic {
		if err := config.SetPresubmitRegexes(presubmits); err != nil {
			t.Fatalf("could not set presubmit regexes for old config: %v", err)
		}
	}
	if err := yaml.Unmarshal([]byte(newConfigData), &newConfig); err != nil {
		t.Fatalf("could not unmarshal new config: %v", err)
	}
	for _, presubmits := range newConfig.PresubmitsStatic {
		if err := config.SetPresubmitRegexes(presubmits); err != nil {
			t.Fatalf("could not set presubmit regexes for new config: %v", err)
		}
	}
	delta := config.Delta{Before: oldConfig, After: newConfig}
	migrate := migration{from: "other-required-job", to: "new-context"}
	org, repo := "org", "repo"
	orgRepoKey := orgRepo{org: org, repo: repo}
	prNumber := 1
	secondPrNumber := 2
	thirdPrNumber := 3
	author := "user"
	prAuthorKey := prAuthor{author: author, pr: prNumber}
	secondPrAuthorKey := prAuthor{author: author, pr: secondPrNumber}
	thirdPrAuthorKey := prAuthor{author: author, pr: thirdPrNumber}
	prOrgRepoKey := prKey{org: org, repo: repo, num: prNumber}
	thirdPrOrgRepoKey := prKey{org: org, repo: repo, num: thirdPrNumber}
	baseRef := "base"
	otherBaseRef := "other"
	baseSha := "abc"
	notMergable := false
	pr := github.PullRequest{
		User: github.User{
			Login: author,
		},
		Number: prNumber,
		Base: github.PullRequestBranch{
			Repo: github.Repo{
				Owner: github.User{
					Login: org,
				},
				Name: repo,
			},
			Ref: baseRef,
		},
		Head: github.PullRequestBranch{
			SHA: "prsha",
		},
	}
	secondPr := github.PullRequest{
		User: github.User{
			Login: author,
		},
		Number: secondPrNumber,
		Base: github.PullRequestBranch{
			Repo: github.Repo{
				Owner: github.User{
					Login: org,
				},
				Name: repo,
			},
			Ref: baseRef,
		},
		Head: github.PullRequestBranch{
			SHA: "prsha2",
		},
		Mergable: &notMergable,
	}
	thirdPr := github.PullRequest{
		User: github.User{
			Login: author,
		},
		Number: thirdPrNumber,
		Base: github.PullRequestBranch{
			Repo: github.Repo{
				Owner: github.User{
					Login: org,
				},
				Name: repo,
			},
			Ref: otherBaseRef,
		},
		Head: github.PullRequestBranch{
			SHA: "prsha3",
		},
	}
	var testCases = []struct {
		name string
		// generator creates the controller and a func that checks
		// the internal state of the fakes in the controller
		generator func() (Controller, func(*testing.T))
		expectErr bool
	}{
		{
			name: "ignored org skips creation, retire and migrate",
			generator: func() (Controller, func(*testing.T)) {
				fpjt := newfakeProwJobTriggerer()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][prAuthorKey] = true
				controller := Controller{
					continueOnError:        true,
					addedPresubmitDenylist: sets.NewString("org"),
					prowJobTriggerer:       &fpjt,
					githubClient:           &fghc,
					statusMigrator:         &fsm,
					trustedChecker:         &ftc,
				}
				checker := func(t *testing.T) {
					checkTriggerer(t, fpjt, map[prKey]sets.String{})
					checkMigrator(t, fsm, map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}, map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}})
				}
				return controller, checker
			},
		},
		{
			name: "ignored org/repo skips creation, retire and migrate",
			generator: func() (Controller, func(*testing.T)) {
				fpjt := newfakeProwJobTriggerer()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][prAuthorKey] = true
				controller := Controller{
					continueOnError:        true,
					addedPresubmitDenylist: sets.NewString("org/repo"),
					prowJobTriggerer:       &fpjt,
					githubClient:           &fghc,
					statusMigrator:         &fsm,
					trustedChecker:         &ftc,
				}
				checker := func(t *testing.T) {
					checkTriggerer(t, fpjt, map[prKey]sets.String{})
					checkMigrator(t, fsm, map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}, map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}})
				}
				return controller, checker
			},
		},
		{
			name: "ignored all org skips creation, retire and migrate",
			generator: func() (Controller, func(*testing.T)) {
				fpjt := newfakeProwJobTriggerer()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][prAuthorKey] = true
				controller := Controller{
					continueOnError:           true,
					addedPresubmitDenylistAll: sets.NewString("org"),
					prowJobTriggerer:          &fpjt,
					githubClient:              &fghc,
					statusMigrator:            &fsm,
					trustedChecker:            &ftc,
				}
				checker := func(t *testing.T) {
					checkTriggerer(t, fpjt, map[prKey]sets.String{})
					checkMigrator(t, fsm, map[orgRepo]sets.String{orgRepoKey: sets.NewString()}, map[orgRepo]migrationSet{orgRepoKey: {}})
				}
				return controller, checker
			},
		},
		{
			name: "ignored all org/repo skips creation, retire and migrate",
			generator: func() (Controller, func(*testing.T)) {
				fpjt := newfakeProwJobTriggerer()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][prAuthorKey] = true
				controller := Controller{
					continueOnError:           true,
					addedPresubmitDenylistAll: sets.NewString("org/repo"),
					prowJobTriggerer:          &fpjt,
					githubClient:              &fghc,
					statusMigrator:            &fsm,
					trustedChecker:            &ftc,
				}
				checker := func(t *testing.T) {
					checkTriggerer(t, fpjt, map[prKey]sets.String{})
					checkMigrator(t, fsm, map[orgRepo]sets.String{orgRepoKey: sets.NewString()}, map[orgRepo]migrationSet{orgRepoKey: {}})
				}
				return controller, checker
			},
		},
		{
			name: "no errors and trusted PR means we should see a trigger, retire and migrate",
			generator: func() (Controller, func(*testing.T)) {
				fpjt := newfakeProwJobTriggerer()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][prAuthorKey] = true
				controller := Controller{
					continueOnError:        true,
					addedPresubmitDenylist: sets.NewString(),
					prowJobTriggerer:       &fpjt,
					githubClient:           &fghc,
					statusMigrator:         &fsm,
					trustedChecker:         &ftc,
				}
				checker := func(t *testing.T) {
					expectedProwJob := map[prKey]sets.String{prOrgRepoKey: sets.NewString("new-required-job")}
					checkTriggerer(t, fpjt, expectedProwJob)
					checkMigrator(t, fsm, map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}, map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}})
				}
				return controller, checker
			},
		},
		{
			name: "no errors and untrusted PR means we should see no trigger, a retire and a migrate",
			generator: func() (Controller, func(*testing.T)) {
				fpjt := newfakeProwJobTriggerer()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][prAuthorKey] = false
				controller := Controller{
					continueOnError:        true,
					addedPresubmitDenylist: sets.NewString(),
					prowJobTriggerer:       &fpjt,
					githubClient:           &fghc,
					statusMigrator:         &fsm,
					trustedChecker:         &ftc,
				}
				checker := func(t *testing.T) {
					checkTriggerer(t, fpjt, map[prKey]sets.String{})
					checkMigrator(t, fsm, map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}, map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}})
				}
				return controller, checker
			},
		},
		{
			name: "no errors and unmergable PR means we should see no trigger, a retire and a migrate",
			generator: func() (Controller, func(*testing.T)) {
				fpjt := newfakeProwJobTriggerer()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{secondPr}
				fghc.refs[orgRepoKey]["heads/"+secondPr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][secondPrAuthorKey] = true
				controller := Controller{
					continueOnError:        true,
					addedPresubmitDenylist: sets.NewString(),
					prowJobTriggerer:       &fpjt,
					githubClient:           &fghc,
					statusMigrator:         &fsm,
					trustedChecker:         &ftc,
				}
				checker := func(t *testing.T) {
					checkTriggerer(t, fpjt, map[prKey]sets.String{})
					checkMigrator(t, fsm, map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}, map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}})
				}
				return controller, checker
			},
		},
		{
			name: "no errors and PR that doesn't match the added job means we should see no trigger, a retire and a migrate",
			generator: func() (Controller, func(*testing.T)) {
				fpjt := newfakeProwJobTriggerer()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{thirdPr}
				fghc.refs[orgRepoKey]["heads/"+thirdPr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][thirdPrAuthorKey] = true
				controller := Controller{
					continueOnError:        true,
					addedPresubmitDenylist: sets.NewString(),
					prowJobTriggerer:       &fpjt,
					githubClient:           &fghc,
					statusMigrator:         &fsm,
					trustedChecker:         &ftc,
				}
				checker := func(t *testing.T) {
					checkTriggerer(t, fpjt, map[prKey]sets.String{thirdPrOrgRepoKey: sets.NewString()})
					checkMigrator(t, fsm, map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}, map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}})
				}
				return controller, checker
			},
		},
		{
			name: "trust check error means we should see no trigger, a retire and a migrate",
			generator: func() (Controller, func(*testing.T)) {
				fpjt := newfakeProwJobTriggerer()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.errors = map[orgRepo]prAuthorSet{orgRepoKey: {prAuthorKey: nil}}
				controller := Controller{
					continueOnError:        true,
					addedPresubmitDenylist: sets.NewString(),
					prowJobTriggerer:       &fpjt,
					githubClient:           &fghc,
					statusMigrator:         &fsm,
					trustedChecker:         &ftc,
				}
				checker := func(t *testing.T) {
					checkTriggerer(t, fpjt, map[prKey]sets.String{})
					checkMigrator(t, fsm, map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}, map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}})
				}
				return controller, checker
			},
			expectErr: true,
		},
		{
			name: "trigger error means we should see no trigger, a retire and a migrate",
			generator: func() (Controller, func(*testing.T)) {
				fpjt := newfakeProwJobTriggerer()
				fpjt.errors[prOrgRepoKey] = sets.NewString("new-required-job")
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.errors = map[orgRepo]prAuthorSet{orgRepoKey: {prAuthorKey: nil}}
				controller := Controller{
					continueOnError:        true,
					addedPresubmitDenylist: sets.NewString(),
					prowJobTriggerer:       &fpjt,
					githubClient:           &fghc,
					statusMigrator:         &fsm,
					trustedChecker:         &ftc,
				}
				checker := func(t *testing.T) {
					checkTriggerer(t, fpjt, map[prKey]sets.String{})
					checkMigrator(t, fsm, map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}, map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}})
				}
				return controller, checker
			},
			expectErr: true,
		},
		{
			name: "retire errors and trusted PR means we should see a trigger and migrate",
			generator: func() (Controller, func(*testing.T)) {
				fpjt := newfakeProwJobTriggerer()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				fsm.retireErrors = map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][prAuthorKey] = true
				controller := Controller{
					continueOnError:        true,
					addedPresubmitDenylist: sets.NewString(),
					prowJobTriggerer:       &fpjt,
					githubClient:           &fghc,
					statusMigrator:         &fsm,
					trustedChecker:         &ftc,
				}
				checker := func(t *testing.T) {
					expectedProwJob := map[prKey]sets.String{prOrgRepoKey: sets.NewString("new-required-job")}
					checkTriggerer(t, fpjt, expectedProwJob)
					checkMigrator(t, fsm, map[orgRepo]sets.String{orgRepoKey: sets.NewString()}, map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}})
				}
				return controller, checker
			},
			expectErr: true,
		},
		{
			name: "migrate errors and trusted PR means we should see a trigger and retire",
			generator: func() (Controller, func(*testing.T)) {
				fpjt := newfakeProwJobTriggerer()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				fsm.migrateErrors = map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}}
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][prAuthorKey] = true
				controller := Controller{
					continueOnError:        true,
					addedPresubmitDenylist: sets.NewString(),
					prowJobTriggerer:       &fpjt,
					githubClient:           &fghc,
					statusMigrator:         &fsm,
					trustedChecker:         &ftc,
				}
				checker := func(t *testing.T) {
					expectedProwJob := map[prKey]sets.String{prOrgRepoKey: sets.NewString("new-required-job")}
					checkTriggerer(t, fpjt, expectedProwJob)
					checkMigrator(t, fsm, map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}, map[orgRepo]migrationSet{orgRepoKey: {}})
				}
				return controller, checker
			},
			expectErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			controller, check := testCase.generator()
			err := controller.reconcile(delta, logrusEntry())
			if err == nil && testCase.expectErr {
				t.Errorf("expected an error, but got none")
			}
			if err != nil && !testCase.expectErr {
				t.Errorf("expected no error, but got one: %v", err)
			}
			check(t)
		})
	}
}

func logrusEntry() *logrus.Entry {
	return logrus.NewEntry(logrus.StandardLogger())
}

func checkTriggerer(t *testing.T, triggerer fakeProwJobTriggerer, expectedCreatedJobs map[prKey]sets.String) {
	if actual, expected := triggerer.created, expectedCreatedJobs; !reflect.DeepEqual(actual, expected) {
		t.Errorf("did not create expected ProwJob: %s", diff.ObjectReflectDiff(actual, expected))
	}
}

func checkMigrator(t *testing.T, migrator fakeMigrator, expectedRetiredStatuses map[orgRepo]sets.String, expectedMigratedStatuses map[orgRepo]migrationSet) {
	if actual, expected := migrator.retired, expectedRetiredStatuses; !reflect.DeepEqual(actual, expected) {
		t.Errorf("did not retire correct statuses: %s", diff.ObjectReflectDiff(actual, expected))
	}
	if actual, expected := migrator.migrated, expectedMigratedStatuses; !reflect.DeepEqual(actual, expected) {
		t.Errorf("did not migrate correct statuses: %s", diff.ObjectReflectDiff(actual, expected))
	}
}
