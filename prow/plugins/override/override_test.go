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
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
)

const (
	fakeOrg     = "fake-org"
	fakeRepo    = "fake-repo"
	fakePR      = 33
	fakeSHA     = "deadbeef"
	fakeBaseSHA = "fffffff"
	adminUser   = "admin-user"
)

type fakeClient struct {
	comments   []string
	statuses   map[string]github.Status
	presubmits map[string]config.Presubmit
	jobs       sets.String
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
		return nil, errors.New("injected CreateStatus failure")
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
		return true, errors.New("injected HasRole error")
	}
	return user == adminUser, nil
}

func (c *fakeClient) GetRef(org, repo, ref string) (string, error) {
	if repo == "fail-ref" {
		return "", errors.New("injected GetRef error")
	}
	return fakeBaseSHA, nil
}

func (c *fakeClient) CreateProwJob(pj kube.ProwJob) (kube.ProwJob, error) {
	if s := pj.Status.State; s != kube.SuccessState {
		return pj, fmt.Errorf("bad status state: %s", s)
	}
	if pj.Spec.Context == "fail-create" {
		return pj, errors.New("injected CreateProwJob error")
	}
	c.jobs.Insert(pj.Spec.Context)
	return pj, nil
}

func (c *fakeClient) presubmitForContext(org, repo, context string) *config.Presubmit {
	p, ok := c.presubmits[context]
	if !ok {
		return nil
	}
	return &p
}

func TestAuthorized(t *testing.T) {
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
			if actual := authorized(&fakeClient{}, log, fakeOrg, fakeRepo, tc.user); actual != tc.expected {
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
					Context: "prow-job",
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
			fc := fakeClient{
				statuses:   tc.contexts,
				presubmits: tc.presubmits,
				jobs:       sets.String{},
			}

			if tc.jobs == nil {
				tc.jobs = sets.String{}
			}

			err := handle(&fc, log, &event)
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
