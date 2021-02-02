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

package override

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pkg/layeredsets"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/ownersconfig"
	"k8s.io/test-infra/prow/repoowners"
)

const (
	fakeOrg     = "fake-org"
	fakeRepo    = "fake-repo"
	fakePR      = 33
	fakeSHA     = "deadbeef"
	fakeBaseSHA = "fffffff"
	adminUser   = "admin-user"
)

type fakeRepoownersClient struct {
	foc *fakeOwnersClient
}

func (froc *fakeRepoownersClient) LoadRepoOwners(org, repo, base string) (repoowners.RepoOwner, error) {
	return froc.foc, nil
}

type fakeOwnersClient struct {
	topLevelApprovers sets.String
}

func (foc *fakeOwnersClient) Filenames() ownersconfig.Filenames {
	return ownersconfig.FakeFilenames
}

func (foc *fakeOwnersClient) TopLevelApprovers() sets.String {
	return foc.topLevelApprovers
}

func (foc *fakeOwnersClient) Approvers(path string) layeredsets.String {
	return layeredsets.String{}
}

func (foc *fakeOwnersClient) LeafApprovers(path string) sets.String {
	return sets.String{}
}

func (foc *fakeOwnersClient) FindApproverOwnersForFile(path string) string {
	return ""
}

func (foc *fakeOwnersClient) Reviewers(path string) layeredsets.String {
	return layeredsets.String{}
}

func (foc *fakeOwnersClient) RequiredReviewers(path string) sets.String {
	return sets.String{}
}

func (foc *fakeOwnersClient) LeafReviewers(path string) sets.String {
	return sets.String{}
}

func (foc *fakeOwnersClient) FindReviewersOwnersForFile(path string) string {
	return ""
}

func (foc *fakeOwnersClient) FindLabelsForFile(path string) sets.String {
	return sets.String{}
}

func (foc *fakeOwnersClient) IsNoParentOwners(path string) bool {
	return false
}

func (foc *fakeOwnersClient) ParseSimpleConfig(path string) (repoowners.SimpleConfig, error) {
	return repoowners.SimpleConfig{}, nil
}

func (foc *fakeOwnersClient) ParseFullConfig(path string) (repoowners.FullConfig, error) {
	return repoowners.FullConfig{}, nil
}

type fakeClient struct {
	comments []string
	statuses map[string]github.Status
	ps       map[string]config.Presubmit
	jobs     sets.String
	owners   ownersClient
}

func (c *fakeClient) presubmits(_, _ string, _ config.RefGetter, _ string) ([]config.Presubmit, error) {
	var result []config.Presubmit
	for _, p := range c.ps {
		result = append(result, p)
	}
	return result, nil
}

func (c *fakeClient) CreateComment(org, repo string, number int, comment string) error {
	switch {
	case org != fakeOrg:
		return fmt.Errorf("bad org: %s", org)
	case repo != fakeRepo:
		return fmt.Errorf("bad repo: %s", repo)
	case number != fakePR:
		return fmt.Errorf("bad number: %d", number)
	case strings.Contains(comment, "fail-comment"):
		return errors.New("injected CreateComment failure")
	}
	c.comments = append(c.comments, comment)
	return nil
}

func (c *fakeClient) CreateStatus(org, repo, ref string, s github.Status) error {
	switch {
	case s.Context == "fail-create":
		return errors.New("injected CreateStatus failure")
	case org != fakeOrg:
		return fmt.Errorf("bad org: %s", org)
	case repo != fakeRepo:
		return fmt.Errorf("bad repo: %s", repo)
	case ref != fakeSHA:
		return fmt.Errorf("bad ref: %s", ref)
	}
	c.statuses[s.Context] = s
	return nil
}

func (c *fakeClient) GetPullRequest(org, repo string, number int) (*github.PullRequest, error) {
	switch {
	case number < 0:
		return nil, errors.New("injected GetPullRequest failure")
	case org != fakeOrg:
		return nil, fmt.Errorf("bad org: %s", org)
	case repo != fakeRepo:
		return nil, fmt.Errorf("bad repo: %s", repo)
	case number != fakePR:
		return nil, fmt.Errorf("bad number: %d", number)
	}
	var pr github.PullRequest
	pr.Head.SHA = fakeSHA
	return &pr, nil
}

func (c *fakeClient) ListStatuses(org, repo, ref string) ([]github.Status, error) {
	switch {
	case org != fakeOrg:
		return nil, fmt.Errorf("bad org: %s", org)
	case repo != fakeRepo:
		return nil, fmt.Errorf("bad repo: %s", repo)
	case ref != fakeSHA:
		return nil, fmt.Errorf("bad ref: %s", ref)
	}
	var out []github.Status
	for _, s := range c.statuses {
		if s.Context == "fail-list" {
			return nil, errors.New("injected ListStatuses failure")
		}
		out = append(out, s)
	}
	return out, nil
}

func (c *fakeClient) HasPermission(org, repo, user string, roles ...string) (bool, error) {
	switch {
	case org != fakeOrg:
		return false, fmt.Errorf("bad org: %s", org)
	case repo != fakeRepo:
		return false, fmt.Errorf("bad repo: %s", repo)
	case roles[0] != github.RoleAdmin:
		return false, fmt.Errorf("bad roles: %s", roles)
	case user == "fail":
		return true, errors.New("injected HasPermission error")
	}
	return user == adminUser, nil
}

func (c *fakeClient) GetRef(org, repo, ref string) (string, error) {
	if repo == "fail-ref" {
		return "", errors.New("injected GetRef error")
	}
	return fakeBaseSHA, nil
}

func (c *fakeClient) ListTeams(org string) ([]github.Team, error) {
	if org == fakeOrg {
		return []github.Team{
			{
				ID:   1,
				Name: "team foo",
				Slug: "team-foo",
			},
		}, nil
	}
	return []github.Team{}, nil
}

func (c *fakeClient) ListTeamMembers(org string, id int, role string) ([]github.TeamMember, error) {
	if id == 1 {
		return []github.TeamMember{
			{Login: "user1"},
			{Login: "user2"},
		}, nil
	}
	return []github.TeamMember{}, nil
}

func (c *fakeClient) Create(_ context.Context, pj *prowapi.ProwJob, _ metav1.CreateOptions) (*prowapi.ProwJob, error) {
	if s := pj.Status.State; s != prowapi.SuccessState {
		return pj, fmt.Errorf("bad status state: %s", s)
	}
	if pj.Spec.Context == "fail-create" {
		return pj, errors.New("injected CreateProwJob error")
	}
	c.jobs.Insert(pj.Spec.Context)
	return pj, nil
}

func (c *fakeClient) LoadRepoOwners(org, repo, base string) (repoowners.RepoOwner, error) {
	return c.owners.LoadRepoOwners(org, repo, base)
}

func TestAuthorizedUser(t *testing.T) {
	cases := []struct {
		name     string
		user     string
		expected bool
	}{
		{
			name: "fail closed",
			user: "fail",
		},
		{
			name: "reject rando",
			user: "random",
		},
		{
			name:     "accept admin",
			user:     adminUser,
			expected: true,
		},
	}

	log := logrus.WithField("plugin", pluginName)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := authorizedUser(&fakeClient{}, log, fakeOrg, fakeRepo, tc.user); actual != tc.expected {
				t.Errorf("actual %t != expected %t", actual, tc.expected)
			}
		})
	}
}

func TestHandle(t *testing.T) {
	cases := []struct {
		name          string
		action        github.GenericCommentEventAction
		issue         bool
		state         string
		comment       string
		contexts      map[string]github.Status
		presubmits    map[string]config.Presubmit
		user          string
		number        int
		expected      map[string]github.Status
		jobs          sets.String
		checkComments []string
		options       plugins.Override
		approvers     []string
		err           bool
	}{
		{
			name:    "successfully override failure",
			comment: "/override broken-test",
			contexts: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusFailure,
				},
			},
			expected: map[string]github.Status{
				"broken-test": {
					Context:     "broken-test",
					Description: description(adminUser),
					State:       github.StatusSuccess,
				},
			},
			checkComments: []string{"on behalf of " + adminUser},
		},
		{
			name:    "successfully override pending",
			comment: "/override hung-test",
			contexts: map[string]github.Status{
				"hung-test": {
					Context: "hung-test",
					State:   github.StatusPending,
				},
			},
			expected: map[string]github.Status{
				"hung-test": {
					Context:     "hung-test",
					Description: description(adminUser),
					State:       github.StatusSuccess,
				},
			},
		},
		{
			name:    "comment for incorrect context",
			comment: "/override whatever-you-want",
			contexts: map[string]github.Status{
				"hung-test": {
					Context: "hung-test",
					State:   github.StatusPending,
				},
			},
			expected: map[string]github.Status{
				"hung-test": {
					Context: "hung-test",
					State:   github.StatusPending,
				},
			},
			checkComments: []string{
				"The following unknown contexts were given", "whatever-you-want",
				"Only the following contexts were expected", "hung-context",
			},
		},
		{
			name:    "refuse override from non-admin",
			comment: "/override broken-test",
			contexts: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusPending,
				},
			},
			user:          "rando",
			checkComments: []string{"unauthorized"},
			expected: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusPending,
				},
			},
		},
		{
			name:    "comment for override with no target",
			comment: "/override",
			contexts: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusPending,
				},
			},
			user:          "rando",
			checkComments: []string{"but none was given"},
			expected: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusPending,
				},
			},
		},
		{
			name:    "override multiple",
			comment: "/override broken-test\n/override hung-test",
			contexts: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusFailure,
				},
				"hung-test": {
					Context: "hung-test",
					State:   github.StatusPending,
				},
			},
			expected: map[string]github.Status{
				"hung-test": {
					Context:     "hung-test",
					Description: description(adminUser),
					State:       github.StatusSuccess,
				},
				"broken-test": {
					Context:     "broken-test",
					Description: description(adminUser),
					State:       github.StatusSuccess,
				},
			},
			checkComments: []string{fmt.Sprintf("%s: broken-test, hung-test", adminUser)},
		},
		{
			name: "override with extra whitespace",
			// Note two spaces here to start, and trailing whitespace
			comment: "/override  broken-test \n",
			contexts: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusFailure,
				},
			},
			expected: map[string]github.Status{
				"broken-test": {
					Context:     "broken-test",
					Description: description(adminUser),
					State:       github.StatusSuccess,
				},
			},
			checkComments: []string{fmt.Sprintf("%s: broken-test", adminUser)},
		},
		{
			name:    "ignore non-PRs",
			issue:   true,
			comment: "/override broken-test",
			contexts: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusPending,
				},
			},
			expected: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusPending,
				},
			},
		},
		{
			name:    "ignore closed issues",
			state:   "closed",
			comment: "/override broken-test",
			contexts: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusPending,
				},
			},
			expected: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusPending,
				},
			},
		},
		{
			name:    "ignore edits",
			action:  github.GenericCommentActionEdited,
			comment: "/override broken-test",
			contexts: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusPending,
				},
			},
			expected: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusPending,
				},
			},
		},
		{
			name:    "ignore random text",
			comment: "/test broken-test",
			contexts: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusPending,
				},
			},
			expected: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusPending,
				},
			},
		},
		{
			name:    "comment on get pr failure",
			number:  fakePR * 2,
			comment: "/override broken-test",
			contexts: map[string]github.Status{
				"broken-test": {
					Context: "broken-test",
					State:   github.StatusFailure,
				},
			},
			expected: map[string]github.Status{
				"broken-test": {
					Context:     "broken-test",
					Description: description(adminUser),
					State:       github.StatusSuccess,
				},
			},
			checkComments: []string{"Cannot get PR"},
		},
		{
			name:    "comment on list statuses failure",
			comment: "/override fail-list",
			contexts: map[string]github.Status{
				"fail-list": {
					Context: "fail-list",
					State:   github.StatusFailure,
				},
			},
			expected: map[string]github.Status{
				"fail-list": {
					Context: "fail-list",
					State:   github.StatusFailure,
				},
			},
			checkComments: []string{"Cannot get commit statuses"},
		},
		{
			name:    "do not override passing contexts",
			comment: "/override passing-test",
			contexts: map[string]github.Status{
				"passing-test": {
					Context:     "passing-test",
					Description: "preserve description",
					State:       github.StatusSuccess,
				},
			},
			expected: map[string]github.Status{
				"passing-test": {
					Context:     "passing-test",
					State:       github.StatusSuccess,
					Description: "preserve description",
				},
			},
		},
		{
			name:    "create successful prow job",
			comment: "/override prow-job",
			contexts: map[string]github.Status{
				"prow-job": {
					Context:     "prow-job",
					Description: "failed",
					State:       github.StatusFailure,
				},
			},
			presubmits: map[string]config.Presubmit{
				"prow-job": {
					Reporter: config.Reporter{
						Context: "prow-job",
					},
				},
			},
			jobs: sets.NewString("prow-job"),
			expected: map[string]github.Status{
				"prow-job": {
					Context:     "prow-job",
					State:       github.StatusSuccess,
					Description: description(adminUser),
				},
			},
		},
		{
			name:    "override with explanation works",
			comment: "/override job\r\nobnoxious flake", // github ends lines with \r\n
			contexts: map[string]github.Status{
				"job": {
					Context:     "job",
					Description: "failed",
					State:       github.StatusFailure,
				},
			},
			expected: map[string]github.Status{
				"job": {
					Context:     "job",
					Description: description(adminUser),
					State:       github.StatusSuccess,
				},
			},
		},
		{
			name:      "override with allow_top_level_owners works",
			comment:   "/override job",
			user:      "code_owner",
			options:   plugins.Override{AllowTopLevelOwners: true},
			approvers: []string{"code_owner"},
			contexts: map[string]github.Status{
				"job": {
					Context:     "job",
					Description: "failed",
					State:       github.StatusFailure,
				},
			},
			expected: map[string]github.Status{
				"job": {
					Context:     "job",
					Description: description("code_owner"),
					State:       github.StatusSuccess,
				},
			},
		},
		{
			name:      "override with allow_top_level_owners works for uppercase user",
			comment:   "/override job",
			user:      "Code_owner",
			options:   plugins.Override{AllowTopLevelOwners: true},
			approvers: []string{"code_owner"},
			contexts: map[string]github.Status{
				"job": {
					Context:     "job",
					Description: "failed",
					State:       github.StatusFailure,
				},
			},
			expected: map[string]github.Status{
				"job": {
					Context:     "job",
					Description: description("Code_owner"),
					State:       github.StatusSuccess,
				},
			},
		},
		{
			name:    "override with allow_top_level_owners fails if user is not in OWNERS file",
			comment: "/override job",
			user:    "non_code_owner",
			options: plugins.Override{AllowTopLevelOwners: true},
			contexts: map[string]github.Status{
				"job": {
					Context:     "job",
					Description: "failed",
					State:       github.StatusFailure,
				},
			},
			expected: map[string]github.Status{
				"job": {
					Context:     "job",
					Description: "failed",
					State:       github.StatusFailure,
				},
			},
		},
		{
			name:    "override with allowed_github_team allowed if user is in specified github team",
			comment: "/override job",
			user:    "user1",
			options: plugins.Override{
				AllowedGitHubTeams: map[string][]string{
					fmt.Sprintf("%s/%s", fakeOrg, fakeRepo): {"team-foo"},
				},
			},
			contexts: map[string]github.Status{
				"job": {
					Context:     "job",
					Description: "failed",
					State:       github.StatusFailure,
				},
			},
			expected: map[string]github.Status{
				"job": {
					Context:     "job",
					Description: description("user1"),
					State:       github.StatusSuccess,
				},
			},
		},
		{
			name:    "override does not fail due to invalid github team slug",
			comment: "/override job",
			user:    "user1",
			options: plugins.Override{
				AllowedGitHubTeams: map[string][]string{
					fmt.Sprintf("%s/%s", fakeOrg, fakeRepo): {"team-foo", "invalid-team-slug"},
				},
			},
			contexts: map[string]github.Status{
				"job": {
					Context:     "job",
					Description: "failed",
					State:       github.StatusFailure,
				},
			},
			expected: map[string]github.Status{
				"job": {
					Context:     "job",
					Description: description("user1"),
					State:       github.StatusSuccess,
				},
			},
		},
	}

	log := logrus.WithField("plugin", pluginName)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var event github.GenericCommentEvent
			event.Repo.Owner.Login = fakeOrg
			event.Repo.Name = fakeRepo
			event.Body = tc.comment
			event.Number = fakePR
			event.IsPR = !tc.issue
			if tc.user == "" {
				tc.user = adminUser
			}
			event.User.Login = tc.user
			if tc.state == "" {
				tc.state = "open"
			}
			event.IssueState = tc.state
			if tc.action == "" {
				tc.action = github.GenericCommentActionCreated
			}
			event.Action = tc.action
			if tc.contexts == nil {
				tc.contexts = map[string]github.Status{}
			}
			froc := &fakeRepoownersClient{
				foc: &fakeOwnersClient{
					topLevelApprovers: sets.NewString(tc.approvers...),
				},
			}
			fc := fakeClient{
				statuses: tc.contexts,
				ps:       tc.presubmits,
				jobs:     sets.String{},
				owners:   froc,
			}

			if tc.jobs == nil {
				tc.jobs = sets.String{}
			}

			err := handle(&fc, log, &event, tc.options)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("unexpected error: %v", err)
				}
			case tc.err:
				t.Error("failed to receive an error")
			case !reflect.DeepEqual(fc.statuses, tc.expected):
				t.Errorf("bad statuses: actual %#v != expected %#v", fc.statuses, tc.expected)
			case !reflect.DeepEqual(fc.jobs, tc.jobs):
				t.Errorf("bad jobs: actual %#v != expected %#v", fc.jobs, tc.jobs)
			}
		})
	}
}

func TestHelpProvider(t *testing.T) {
	cases := []struct {
		name        string
		config      plugins.Configuration
		org         string
		repo        string
		expectedWho string
	}{
		{
			name:        "WhoCanUse restricted to Repo administrators if no other options specified",
			config:      plugins.Configuration{},
			expectedWho: "Repo administrators.",
		},
		{
			name: "WhoCanUse includes top level code OWNERS if allow_top_level_owners is set",
			config: plugins.Configuration{
				Override: plugins.Override{
					AllowTopLevelOwners: true,
				},
			},
			expectedWho: "Repo administrators, approvers in top level OWNERS file.",
		},
		{
			name: "WhoCanUse includes specified github teams",
			config: plugins.Configuration{
				Override: plugins.Override{
					AllowedGitHubTeams: map[string][]string{
						"org1/repo1": {"team-foo", "team-bar"},
					},
				},
			},
			expectedWho: "Repo administrators, and the following github teams:" +
				"org1/repo1: team-foo team-bar.",
		},
	}

	for _, tc := range cases {
		help, err := helpProvider(&tc.config, []config.OrgRepo{})
		if err != nil {
			t.Errorf("%s: unexpected error: %v", tc.name, err)
		}
		switch {
		case help == nil:
			t.Errorf("%s: expected a valid plugin help object, got nil", tc.name)
		case len(help.Commands) != 1:
			t.Errorf("%s: expected a single command from plugin help, got: %v", tc.name, help.Commands)
		case help.Commands[0].WhoCanUse != tc.expectedWho:
			t.Errorf("%s: expected a single command with WhoCanUse set to %s, got %s instead", tc.name, tc.expectedWho, help.Commands[0].WhoCanUse)
		}
	}
}

func TestWhoCanUse(t *testing.T) {
	override := plugins.Override{
		AllowedGitHubTeams: map[string][]string{
			"org1/repo1": {"team-foo", "team-bar"},
			"org2/repo2": {"team-bar"},
		},
	}
	expectedWho := "Repo administrators, and the following github teams:" +
		"org1/repo1: team-foo team-bar."

	who := whoCanUse(override, "org1", "repo1")
	if who != expectedWho {
		t.Errorf("expected %s, got %s", expectedWho, who)
	}
}

func TestAuthorizedGitHubTeamMember(t *testing.T) {
	repoRef := fmt.Sprintf("%s/%s", fakeOrg, fakeRepo)
	cases := []struct {
		name     string
		slugs    map[string][]string
		org      string
		repo     string
		user     string
		expected bool
	}{
		{
			name: "members of specified teams are authorized",
			slugs: map[string][]string{
				repoRef: {"team-foo"},
			},
			user:     "user1",
			expected: true,
		},
		{
			name: "non-members of specified teams are not authorized",
			slugs: map[string][]string{
				repoRef: {"team-foo"},
			},
			user: "non-member",
		},
		{
			name: "only teams corresponding to the org/repo are considered",
			slugs: map[string][]string{
				"org/repo": {"team-foo"},
			},
			user: "member",
		},
	}
	log := logrus.WithField("plugin", pluginName)
	for _, tc := range cases {
		authorized := authorizedGitHubTeamMember(&fakeClient{}, log, tc.slugs, fakeOrg, fakeRepo, tc.user)
		if authorized != tc.expected {
			t.Errorf("%s: actual: %v != expected %v", tc.name, authorized, tc.expected)
		}
	}
}

func TestValidateGitHubTeamSlugs(t *testing.T) {
	githubTeams := []github.Team{
		{
			ID:   2,
			Slug: "team-foo",
		},
		{
			ID:   3,
			Slug: "team-bar",
		},
	}

	repoRef := fmt.Sprintf("%s/%s", fakeOrg, fakeRepo)
	cases := []struct {
		name      string
		teamSlugs map[string][]string
		err       error
	}{
		{
			name: "validation failure for invalid team slug",
			teamSlugs: map[string][]string{
				repoRef: {"foo"},
			},
			err: fmt.Errorf("invalid team slug(s): foo"),
		},
		{
			name: "no errors for valid team slugs",
			teamSlugs: map[string][]string{
				repoRef: {"team-foo", "team-bar"},
			},
		},
	}

	for _, tc := range cases {
		err := validateGitHubTeamSlugs(tc.teamSlugs, fakeOrg, fakeRepo, githubTeams)
		if !reflect.DeepEqual(err, tc.err) {
			t.Errorf("%s: actual: %v != expected %v", tc.name, err, tc.err)
		}
	}
}
