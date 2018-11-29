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

	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pjutil"
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
  context: old-context`,
			new: `"org/repo":
- name: old-job
  context: old-context`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "added optional presubmit means no added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `"org/repo":
- name: old-job
  context: old-context
- name: new-job
  context: new-context
  optional: true`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "added non-reporting presubmit means no added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `"org/repo":
- name: old-job
  context: old-context
- name: new-job
  context: new-context
  skip_report: true`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "added required presubmit means added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context`,
			new: `"org/repo":
- name: old-job
  context: old-context
- name: new-job
  context: new-context`,
			expected: map[string][]config.Presubmit{
				"org/repo": {{
					JobBase:    config.JobBase{Name: "new-job"},
					Context:    "new-context",
					Optional:   false,
					SkipReport: false,
				}},
			},
		},
		{
			name: "optional presubmit transitioning to required means no added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  optional: true`,
			new: `"org/repo":
- name: old-job
  context: old-context`,
			expected: map[string][]config.Presubmit{
				"org/repo": {},
			},
		},
		{
			name: "non-reporting presubmit transitioning to required means added blocking jobs",
			old: `"org/repo":
- name: old-job
  context: old-context
  skip_report: true`,
			new: `"org/repo":
- name: old-job
  context: old-context`,
			expected: map[string][]config.Presubmit{
				"org/repo": {{
					JobBase: config.JobBase{Name: "old-job"},
					Context: "old-context",
				}},
			},
		},
		{
			name: "required presubmit transitioning to new context means no added blocking jobs",
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
			if actual, expected := addedBlockingPresubmits(oldConfig, newConfig), testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: did not get correct added presubmits: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
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
					JobBase: config.JobBase{Name: "old-job"},
					Context: "old-context",
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
					JobBase: config.JobBase{Name: "old-job"},
					Context: "old-context",
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
			if actual, expected := removedBlockingPresubmits(oldConfig, newConfig), testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: did not get correct removed presubmits: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
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
						JobBase: config.JobBase{Name: "old-job"},
						Context: "old-context",
					},
					to: config.Presubmit{
						JobBase: config.JobBase{Name: "old-job"},
						Context: "new-context",
					},
				}},
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
			if actual, expected := migratedBlockingPresubmits(oldConfig, newConfig), testCase.expected; !reflect.DeepEqual(actual, expected) {
				t.Errorf("%s: did not get correct removed presubmits: %v", testCase.name, diff.ObjectReflectDiff(actual, expected))
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

func (m *fakeMigrator) retire(org, repo, context string) error {
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

func (m *fakeMigrator) migrate(org, repo, from, to string) error {
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

func newfakeKubeClient() fakeKubeClient {
	return fakeKubeClient{
		errors:  sets.NewString(),
		created: []kube.ProwJobSpec{},
	}
}

type fakeKubeClient struct {
	errors  sets.String
	created []kube.ProwJobSpec
}

func (c *fakeKubeClient) CreateProwJob(j kube.ProwJob) (kube.ProwJob, error) {
	if c.errors.Has(j.Name) {
		return j, errors.New("failed to create prow job")
	}
	c.created = append(c.created, j.Spec)
	return j, nil
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
	prErrors  orgRepoSet
	refErrors map[orgRepo]sets.String

	prs  map[orgRepo][]github.PullRequest
	refs map[orgRepo]map[string]string
}

func (c *fakeGitHubClient) GetPullRequests(org, repo string) ([]github.PullRequest, error) {
	key := orgRepo{org: org, repo: repo}
	if c.prErrors.has(key) {
		return nil, errors.New("failed to get PRs")
	}
	return c.prs[key], nil
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
  - name: other-required-job
    context: other-required-job`
	newConfigData := `presubmits:
  "org/repo":
  - name: other-required-job
    context: new-context
  - name: new-required-job
    context: new-required-context`

	var oldConfig, newConfig config.Config
	if err := yaml.Unmarshal([]byte(oldConfigData), &oldConfig); err != nil {
		t.Fatalf("could not unmarshal old config: %v", err)
	}
	if err := yaml.Unmarshal([]byte(newConfigData), &newConfig); err != nil {
		t.Fatalf("could not unmarshal new config: %v", err)
	}
	delta := config.ConfigDelta{Before: oldConfig, After: newConfig}
	migrate := migration{from: "other-required-job", to: "new-context"}
	org, repo := "org", "repo"
	orgRepoKey := orgRepo{org: org, repo: repo}
	prNumber := 1
	author := "user"
	prAuthorKey := prAuthor{author: author, pr: prNumber}
	baseRef := "base"
	baseSha := "abc"
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
	var testCases = []struct {
		name string
		// generator creates the controller and a func that checks
		// the internal state of the fakes in the controller
		generator func() (Controller, func(*testing.T))
		expectErr bool
	}{
		{
			name: "no errors and trusted PR means we should see a trigger, retire and migrate",
			generator: func() (Controller, func(*testing.T)) {
				fkc := newfakeKubeClient()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][prAuthorKey] = true
				return Controller{
						continueOnError: true, kubeClient: &fkc, githubClient: &fghc, statusMigrator: &fsm, trustedChecker: &ftc,
					}, func(t *testing.T) {
						expectedProwJob := pjutil.NewPresubmit(pr, baseSha, config.Presubmit{
							JobBase: config.JobBase{Name: "new-required-job"},
							Context: "new-required-context",
						}, "none").Spec
						if actual, expected := fkc.created, []kube.ProwJobSpec{expectedProwJob}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not create expected ProwJob: %s", diff.ObjectReflectDiff(actual, expected))
						}
						if actual, expected := fsm.retired, map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not retire correct statuses: %s", diff.ObjectReflectDiff(actual, expected))
						}
						if actual, expected := fsm.migrated, map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not migrate correct statuses: %s", diff.ObjectReflectDiff(actual, expected))
						}
					}
			},
		},
		{
			name: "no errors and untrusted PR means we should see no trigger, a retire and a migrate",
			generator: func() (Controller, func(*testing.T)) {
				fkc := newfakeKubeClient()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][prAuthorKey] = false
				return Controller{
						continueOnError: true, kubeClient: &fkc, githubClient: &fghc, statusMigrator: &fsm, trustedChecker: &ftc,
					}, func(t *testing.T) {
						if actual, expected := fkc.created, []kube.ProwJobSpec{}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not create expected ProwJob: %s", diff.ObjectReflectDiff(actual, expected))
						}
						if actual, expected := fsm.retired, map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not retire correct statuses: %s", diff.ObjectReflectDiff(actual, expected))
						}
						if actual, expected := fsm.migrated, map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not migrate correct statuses: %s", diff.ObjectReflectDiff(actual, expected))
						}
					}
			},
		},
		{
			name: "trust check error means we should see no trigger, a retire and a migrate",
			generator: func() (Controller, func(*testing.T)) {
				fkc := newfakeKubeClient()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.errors = map[orgRepo]prAuthorSet{orgRepoKey: {prAuthorKey: nil}}
				return Controller{
						continueOnError: true, kubeClient: &fkc, githubClient: &fghc, statusMigrator: &fsm, trustedChecker: &ftc,
					}, func(t *testing.T) {
						if actual, expected := fkc.created, []kube.ProwJobSpec{}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not create expected ProwJob: %s", diff.ObjectReflectDiff(actual, expected))
						}
						if actual, expected := fsm.retired, map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not retire correct statuses: %s", diff.ObjectReflectDiff(actual, expected))
						}
						if actual, expected := fsm.migrated, map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not migrate correct statuses: %s", diff.ObjectReflectDiff(actual, expected))
						}
					}
			},
			expectErr: true,
		},
		{
			name: "retire errors and trusted PR means we should see a trigger and migrate",
			generator: func() (Controller, func(*testing.T)) {
				fkc := newfakeKubeClient()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				fsm.retireErrors = map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][prAuthorKey] = true
				return Controller{
						continueOnError: true, kubeClient: &fkc, githubClient: &fghc, statusMigrator: &fsm, trustedChecker: &ftc,
					}, func(t *testing.T) {
						expectedProwJob := pjutil.NewPresubmit(pr, baseSha, config.Presubmit{
							JobBase: config.JobBase{Name: "new-required-job"},
							Context: "new-required-context",
						}, "none").Spec
						if actual, expected := fkc.created, []kube.ProwJobSpec{expectedProwJob}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not create expected ProwJob: %s", diff.ObjectReflectDiff(actual, expected))
						}
						if actual, expected := fsm.retired, map[orgRepo]sets.String{orgRepoKey: sets.NewString()}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not retire correct statuses: %s", diff.ObjectReflectDiff(actual, expected))
						}
						if actual, expected := fsm.migrated, map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not migrate correct statuses: %s", diff.ObjectReflectDiff(actual, expected))
						}
					}
			},
			expectErr: true,
		},
		{
			name: "migrate errors and trusted PR means we should see a trigger and retire",
			generator: func() (Controller, func(*testing.T)) {
				fkc := newfakeKubeClient()
				fghc := newFakeGitHubClient(orgRepoKey)
				fghc.prs[orgRepoKey] = []github.PullRequest{pr}
				fghc.refs[orgRepoKey]["heads/"+pr.Base.Ref] = baseSha
				fsm := newFakeMigrator(orgRepoKey)
				fsm.migrateErrors = map[orgRepo]migrationSet{orgRepoKey: {migrate: nil}}
				ftc := newFakeTrustedChecker(orgRepoKey)
				ftc.trusted[orgRepoKey][prAuthorKey] = true
				return Controller{
						continueOnError: true, kubeClient: &fkc, githubClient: &fghc, statusMigrator: &fsm, trustedChecker: &ftc,
					}, func(t *testing.T) {
						expectedProwJob := pjutil.NewPresubmit(pr, baseSha, config.Presubmit{
							JobBase: config.JobBase{Name: "new-required-job"},
							Context: "new-required-context",
						}, "none").Spec
						if actual, expected := fkc.created, []kube.ProwJobSpec{expectedProwJob}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not create expected ProwJob: %s", diff.ObjectReflectDiff(actual, expected))
						}
						if actual, expected := fsm.retired, map[orgRepo]sets.String{orgRepoKey: sets.NewString("required-job")}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not retire correct statuses: %s", diff.ObjectReflectDiff(actual, expected))
						}
						if actual, expected := fsm.migrated, map[orgRepo]migrationSet{orgRepoKey: {}}; !reflect.DeepEqual(actual, expected) {
							t.Errorf("did not migrate correct statuses: %s", diff.ObjectReflectDiff(actual, expected))
						}
					}
			},
			expectErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			controller, check := testCase.generator()
			err := controller.reconcile(delta)
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
