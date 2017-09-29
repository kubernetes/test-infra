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

package ghclient

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-github/github"
)

type fakeUserService struct {
	authenticatedUser string
	users             map[string]*github.User
}

func newFakeUserService(authenticated string, other []string) *fakeUserService {
	users := map[string]*github.User{authenticated: {Login: &authenticated}}
	for _, user := range other {
		userCopy := user
		users[user] = &github.User{Login: &userCopy}
	}
	return &fakeUserService{authenticatedUser: authenticated, users: users}
}

func (f *fakeUserService) Get(ctx context.Context, login string) (*github.User, *github.Response, error) {
	resp := &github.Response{Rate: github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: time.Now()}}}
	if login == "" {
		login = f.authenticatedUser
	}
	if user := f.users[login]; user != nil {
		return user, resp, nil
	}
	return nil, resp, fmt.Errorf("user '%s' does not exist", login)
}

func TestGetUser(t *testing.T) {
	client := &Client{userService: newFakeUserService("me", []string{"a", "b", "c"})}
	setForTest(client)
	// try getting the currently authenticated user
	if user, err := client.GetUser(""); err != nil {
		t.Errorf("Unexpected error from GetUser(\"\"): %v.", err)
	} else if *user.Login != "me" {
		t.Errorf("GetUser(\"\") returned user %q instead of \"me\".", *user.Login)
	}
	// try getting another user
	if user, err := client.GetUser("b"); err != nil {
		t.Errorf("Unexpected error from GetUser(\"b\"): %v.", err)
	} else if *user.Login != "b" {
		t.Errorf("GetUser(\"b\") returned user %q instead of \"b\".", *user.Login)
	}
	// try getting an invalid user
	if user, err := client.GetUser("d"); err == nil {
		t.Errorf("Expected error from GetUser(\"d\") (invalid user), but did not get an error.")
	} else if user != nil {
		t.Error("Got a user even though GetUser(\"d\") (invalid user) returned a nil user.")
	}
}

type fakeRepoService struct {
	org, repo     string
	collaborators []*github.User

	ref         string
	status      *github.RepoStatus
	statusCount int // Number of statuses in combined status.
}

func newFakeRepoService(org, repo, ref string, statuses int, collaborators []string) *fakeRepoService {
	users := make([]*github.User, 0, len(collaborators))
	for _, user := range collaborators {
		userCopy := user
		users = append(users, &github.User{Login: &userCopy})
	}
	return &fakeRepoService{org: org, repo: repo, collaborators: users, ref: ref, statusCount: statuses}
}

func (f *fakeRepoService) CreateStatus(ctx context.Context, org, repo, ref string, status *github.RepoStatus) (*github.RepoStatus, *github.Response, error) {
	resp := &github.Response{
		Rate:     github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: time.Now()}},
		LastPage: 1,
	}
	if org != f.org {
		return nil, resp, fmt.Errorf("org '%s' not recognized, only '%s' is valid", org, f.org)
	}
	if repo != f.repo {
		return nil, resp, fmt.Errorf("repo '%s' not recognized, only '%s' is valid", repo, f.repo)
	}
	if ref != f.ref {
		return nil, resp, fmt.Errorf("ref '%s' not recognized, only '%s' is valid", ref, f.ref)
	}
	f.status = status
	return status, resp, nil
}

func (f *fakeRepoService) GetCombinedStatus(ctx context.Context, org, repo, ref string, opt *github.ListOptions) (*github.CombinedStatus, *github.Response, error) {
	resp := &github.Response{
		Rate:     github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: time.Now()}},
		LastPage: (f.statusCount + 1) / 2,
	}
	if org != f.org {
		return nil, resp, fmt.Errorf("org '%s' not recognized, only '%s' is valid", org, f.org)
	}
	if repo != f.repo {
		return nil, resp, fmt.Errorf("repo '%s' not recognized, only '%s' is valid", repo, f.repo)
	}
	if ref != f.ref {
		return nil, resp, fmt.Errorf("ref '%s' not recognized, only '%s' is valid", ref, f.ref)
	}
	state := "success"
	context1 := fmt.Sprintf("context %d", (opt.Page*2)-1)
	context2 := fmt.Sprintf("context %d", opt.Page*2)
	comb := &github.CombinedStatus{
		SHA:      &ref,
		State:    &state,
		Statuses: []github.RepoStatus{{Context: &context1}, {Context: &context2}},
	}
	return comb, resp, nil
}

// ListCollaborators returns 2 collaborators per page of results (served in order).
func (f *fakeRepoService) ListCollaborators(ctx context.Context, owner, repo string, opt *github.ListOptions) ([]*github.User, *github.Response, error) {
	resp := &github.Response{
		Rate:     github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: time.Now()}},
		LastPage: (len(f.collaborators) + 1) / 2,
	}
	if owner != f.org {
		return nil, resp, fmt.Errorf("org '%s' not recognized, only '%s' is valid", owner, f.org)
	}
	if repo != f.repo {
		return nil, resp, fmt.Errorf("repo '%s' not recognized, only '%s' is valid", repo, f.repo)
	}
	if len(f.collaborators) == 0 {
		return nil, resp, nil
	}
	return []*github.User{f.collaborators[(opt.Page*2)-2], f.collaborators[(opt.Page*2)-1]}, resp, nil
}

func TestCreateStatus(t *testing.T) {
	contextStr := "context"
	stateStr := "some state"
	descStr := "descriptive description"
	urlStr := "link"
	sampleStatus := &github.RepoStatus{
		Context:     &contextStr,
		State:       &stateStr,
		Description: &descStr,
		TargetURL:   &urlStr,
	}
	svc := newFakeRepoService("k8s", "kuber", "ref", 0, nil)
	client := &Client{repoService: svc}
	setForTest(client)
	status, err := client.CreateStatus("k8s", "kuber", "ref", sampleStatus)
	if err != nil {
		t.Fatalf("Unexpected error from CreateStatus with valid args: %v", err)
	}
	if status == nil {
		t.Fatalf("Expected status returned by CreateStatus to be non-nil, but it was nil.")
	}
	if *status.Context != contextStr {
		t.Errorf("Expected RepoStatus from CreateStatus to have a context of '%s' instead of '%s'", contextStr, *status.Context)
	}
	if *status.State != stateStr {
		t.Errorf("Expected RepoStatus from CreateStatus to have a state of '%s' instead of '%s'", stateStr, *status.State)
	}
	if *status.Description != descStr {
		t.Errorf("Expected RepoStatus from CreateStatus to have a description of '%s' instead of '%s'", descStr, *status.Description)
	}
	if *status.TargetURL != urlStr {
		t.Errorf("Expected RepoStatus from CreateStatus to have a target URL of '%s' instead of '%s'", urlStr, *status.TargetURL)
	}

	_, err = client.CreateStatus("k8s", "kuber", "ref2", sampleStatus)
	if err == nil {
		t.Error("Expected error from CreateStatus on invalid ref, but didn't get an error.")
	}
}

func TestGetCombinedStatus(t *testing.T) {
	svc := newFakeRepoService("k8s", "kuber", "ref", 6, nil)
	client := &Client{repoService: svc}
	setForTest(client)
	combStatus, err := client.GetCombinedStatus("k8s", "kuber", "ref")
	if err != nil {
		t.Fatalf("Unexpected error from GetCombinedStatus on valid args: %v", err)
	}
	if combStatus == nil {
		t.Fatal("Expected the combined status to be non-nil!")
	}
	if *combStatus.State != "success" {
		t.Errorf("Expected the combined status to have state 'success', not '%s'.", *combStatus.State)
	}
	if len(combStatus.Statuses) != svc.statusCount {
		t.Errorf("Expected %d statuses in the combined status, not %d.", svc.statusCount, len(combStatus.Statuses))
	}
	for index, status := range combStatus.Statuses {
		if status.Context == nil {
			t.Fatalf("CombinedStatus has status at index %d with a nil 'Context' field.", index)
		}
		expectedContext := fmt.Sprintf("context %d", index+1)
		if *status.Context != expectedContext {
			t.Errorf("Expected status at index %d to have a context of '%s' instead of '%s'.", index, expectedContext, *status.Context)
		}
	}
	if _, err = client.GetCombinedStatus("k8s", "kuber", "ref2"); err == nil {
		t.Error("Expected error getting CombinedStatus for non-existent ref, but got none.")
	}
}

func TestGetCollaborators(t *testing.T) {
	var users []*github.User
	var err error
	// test normal case
	expected := []string{"a", "b", "c", "d"}
	client := &Client{repoService: newFakeRepoService("k8s", "kuber", "", 0, expected)}
	setForTest(client)
	if users, err = client.GetCollaborators("k8s", "kuber"); err != nil {
		t.Errorf("Unexpected error from GetCollaborators on valid org and repo: %v.", err)
	} else {
		for _, expect := range expected {
			found := false
			for _, user := range users {
				if *user.Login == expect {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected to find %q as a collaborator, but did not.", expect)
			}
		}
		if len(users) > len(expected) {
			t.Errorf("Expected to find %d collaborators, but found %d instead.", len(expected), len(users))
		}
	}
	// test invalid repo
	if users, err = client.GetCollaborators("not-an-org", "not a repo"); err == nil {
		t.Error("Expected error from GetCollaborators, but did not get an error.")
	}
	if len(users) > 0 {
		t.Errorf("Received users from GetCollaborators even though it returned an error.")
	}
	// test empty list
	client = &Client{repoService: newFakeRepoService("k8s", "kuber", "", 0, nil)}
	setForTest(client)
	if users, err = client.GetCollaborators("k8s", "kuber"); err != nil {
		t.Errorf("Unexpected error from GetCollaborators on valid org and repo: %v.", err)
	}
	if len(users) > 0 {
		t.Errorf("Received users from GetCollaborators even though there are no collaborators.")
	}
}

type fakeIssueService struct {
	org, repo  string
	repoLabels []*github.Label
	repoIssues map[int]*github.Issue
}

func newFakeIssueService(org, repo string, labels []string, issueCount int) *fakeIssueService {
	repoLabels := make([]*github.Label, 0, len(labels))
	for _, label := range labels {
		labelCopy := label
		repoLabels = append(repoLabels, &github.Label{Name: &labelCopy})
	}
	repoIssues := map[int]*github.Issue{}
	for i := 1; i <= issueCount; i++ {
		iCopy := i
		text := fmt.Sprintf("%d", i)
		issue := &github.Issue{
			Title:     &text,
			Body:      &text,
			Number:    &iCopy,
			Labels:    []github.Label{{Name: &text}},
			Assignees: []*github.User{{Login: &text}},
		}
		repoIssues[i] = issue
	}
	return &fakeIssueService{org: org, repo: repo, repoLabels: repoLabels, repoIssues: repoIssues}
}

func (f *fakeIssueService) Create(ctx context.Context, owner string, repo string, issue *github.IssueRequest) (*github.Issue, *github.Response, error) {
	resp := &github.Response{Rate: github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: time.Now()}}}
	if owner != f.org {
		return nil, resp, fmt.Errorf("org '%s' not recognized, only '%s' is valid", owner, f.org)
	}
	if repo != f.repo {
		return nil, resp, fmt.Errorf("repo '%s' not recognized, only '%s' is valid", repo, f.repo)
	}
	number := len(f.repoIssues) + 1
	result := &github.Issue{
		Title:  issue.Title,
		Body:   issue.Body,
		Number: &number,
	}
	for _, label := range *issue.Labels {
		labelCopy := label
		result.Labels = append(result.Labels, github.Label{Name: &labelCopy})
	}
	for _, user := range *issue.Assignees {
		userCopy := user
		result.Assignees = append(result.Assignees, &github.User{Login: &userCopy})
	}
	f.repoIssues[number] = result
	return result, resp, nil
}

// ListByRepo returns 2 issues per page of results (served in order by number).
func (f *fakeIssueService) ListByRepo(ctx context.Context, org, repo string, opt *github.IssueListByRepoOptions) ([]*github.Issue, *github.Response, error) {
	resp := &github.Response{
		Rate:     github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: time.Now()}},
		LastPage: (len(f.repoIssues) + 1) / 2,
	}
	if org != f.org {
		return nil, resp, fmt.Errorf("org '%s' not recognized, only '%s' is valid", org, f.org)
	}
	if repo != f.repo {
		return nil, resp, fmt.Errorf("repo '%s' not recognized, only '%s' is valid", repo, f.repo)
	}
	if len(f.repoIssues) == 0 {
		return nil, resp, nil
	}
	return []*github.Issue{f.repoIssues[(opt.ListOptions.Page*2)-1], f.repoIssues[opt.ListOptions.Page*2]}, resp, nil
}

// ListLabels returns 2 labels per page or results (served in order).
func (f *fakeIssueService) ListLabels(ctx context.Context, owner, repo string, opt *github.ListOptions) ([]*github.Label, *github.Response, error) {
	resp := &github.Response{
		Rate:     github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: time.Now()}},
		LastPage: (len(f.repoLabels) + 1) / 2,
	}
	if owner != f.org {
		return nil, resp, fmt.Errorf("org '%s' not recognized, only '%s' is valid", owner, f.org)
	}
	if repo != f.repo {
		return nil, resp, fmt.Errorf("repo '%s' not recognized, only '%s' is valid", repo, f.repo)
	}
	if len(f.repoLabels) == 0 {
		return nil, resp, nil
	}
	return []*github.Label{f.repoLabels[(opt.Page*2)-2], f.repoLabels[(opt.Page*2)-1]}, resp, nil
}

func TestCreateIssue(t *testing.T) {
	expectedLabels := []string{"label1", "label2"}
	expectedAssignees := []string{"user1", "user2"}

	svc := newFakeIssueService("k8s", "kuber", nil, 3)
	client := &Client{issueService: svc}
	setForTest(client)
	issue, err := client.CreateIssue("k8s", "kuber", "Title", "Body", expectedLabels, expectedAssignees)
	if err != nil {
		t.Fatalf("Unexpected error from CreateIssue with valid args: %v.", err)
	}
	if issue == nil {
		t.Fatalf("Expected issue returned by CreateIssue to be non-nil, but it was nil.")
	}
	if *issue.Title != "Title" {
		t.Errorf("Expected issue from CreateIssue to have a title of 'Title' instead of '%s'.", *issue.Title)
	}
	if *issue.Body != "Body" {
		t.Errorf("Expected issue from CreateIssue to have a state of 'Body' instead of '%s'.", *issue.Body)
	}
	for _, label := range expectedLabels {
		found := false
		for _, actual := range issue.Labels {
			if *actual.Name == label {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected issue from CreateIssue to have the label '%s'.", label)
		}
	}
	for _, assignee := range expectedAssignees {
		found := false
		for _, actual := range issue.Assignees {
			if *actual.Login == assignee {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected issue from CreateIssue to have the assignee '%s'.", assignee)
		}
	}

	_, err = client.CreateIssue("k8s", "not-a-repo", "Title", "Body", nil, nil)
	if err == nil {
		t.Error("Expected error from CreateIssue on invalid repo, but didn't get an error.")
	}
}

func TestGetIssues(t *testing.T) {
	var issues []*github.Issue
	var err error
	// test normal case
	client := &Client{issueService: newFakeIssueService("k8s", "kuber", nil, 10)}
	setForTest(client)
	if issues, err = client.GetIssues("k8s", "kuber", &github.IssueListByRepoOptions{}); err != nil {
		t.Errorf("Unexpected error from GetIssues on valid org and repo: %v.", err)
	} else {
		for i := 1; i <= 10; i++ {
			found := false
			for _, issue := range issues {
				if *issue.Number == i {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected to find issue #%d, but did not.", i)
			}
		}
		if len(issues) > 10 {
			t.Errorf("Expected to find 10 issues, but found %d instead.", len(issues))
		}
	}
	// test invalid repo
	if issues, err = client.GetIssues("not-an-org", "not a repo", &github.IssueListByRepoOptions{}); err == nil {
		t.Error("Expected error from GetIssues, but did not get an error.")
	}
	if len(issues) > 0 {
		t.Errorf("Received issues from GetIssues even though it returned an error.")
	}
	// test empty list
	client = &Client{issueService: newFakeIssueService("k8s", "kuber", nil, 0)}
	setForTest(client)
	if issues, err = client.GetIssues("k8s", "kuber", &github.IssueListByRepoOptions{}); err != nil {
		t.Errorf("Unexpected error from GetIssues on valid org and repo: %v.", err)
	}
	if len(issues) > 0 {
		t.Errorf("Received %d issues from GetIssues even though there are no issues.", len(issues))
	}
}

func TestGetRepoLabels(t *testing.T) {
	var labels []*github.Label
	var err error
	// test normal case
	expected := []string{"a", "b", "c", "d"}
	client := &Client{issueService: newFakeIssueService("k8s", "kuber", expected, 0)}
	setForTest(client)
	if labels, err = client.GetRepoLabels("k8s", "kuber"); err != nil {
		t.Errorf("Unexpected error from GetRepoLabels on valid org and repo: %v.", err)
	} else {
		for _, expect := range expected {
			found := false
			for _, label := range labels {
				if *label.Name == expect {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected to find %q as a label, but did not.", expect)
			}
		}
		if len(labels) > len(expected) {
			t.Errorf("Expected to find %d labels, but found %d instead.", len(expected), len(labels))
		}
	}
	// test invalid repo
	if labels, err = client.GetRepoLabels("not-an-org", "not a repo"); err == nil {
		t.Error("Expected error from GetRepoLabels, but did not get an error.")
	}
	if len(labels) > 0 {
		t.Errorf("Received labels from GetRepoLabels even though it returned an error.")
	}
	// test empty list
	client = &Client{issueService: newFakeIssueService("k8s", "kuber", nil, 0)}
	setForTest(client)
	if labels, err = client.GetRepoLabels("k8s", "kuber"); err != nil {
		t.Errorf("Unexpected error from GetRepoLabels on valid org and repo: %v.", err)
	}
	if len(labels) > 0 {
		t.Errorf("Received labels from GetRepoLabels even though there are no labels.")
	}
}

type fakePullRequestService struct {
	org, repo string
	prCount   int
}

//	List returns 2 PRs per page of results.
func (f *fakePullRequestService) List(ctx context.Context, org, repo string, opts *github.PullRequestListOptions) ([]*github.PullRequest, *github.Response, error) {
	resp := &github.Response{
		Rate:     github.Rate{Limit: 5000, Remaining: 1000, Reset: github.Timestamp{Time: time.Now()}},
		LastPage: (f.prCount + 1) / 2,
	}
	if org != f.org {
		return nil, resp, fmt.Errorf("org '%s' not recognized, only '%s' is valid", org, f.org)
	}
	if repo != f.repo {
		return nil, resp, fmt.Errorf("repo '%s' not recognized, only '%s' is valid", repo, f.repo)
	}
	title1 := fmt.Sprintf("Title %d", (opts.Page*2)-1)
	title2 := fmt.Sprintf("Title %d", opts.Page*2)
	return []*github.PullRequest{{Title: &title1}, {Title: &title2}}, resp, nil
}

func TestForEachPR(t *testing.T) {
	svc := &fakePullRequestService{org: "k8s", repo: "kuber", prCount: 8}
	client := &Client{prService: svc}
	setForTest(client)

	processed := 0
	process := func(pr *github.PullRequest) error {
		if pr == nil || pr.Title == nil {
			return fmt.Errorf("pr %d was invalid", processed+1)
		}
		expectedTitle := fmt.Sprintf("Title %d", processed+1)
		if *pr.Title != expectedTitle {
			return fmt.Errorf("expected pr title '%s' but got '%s'", expectedTitle, *pr.Title)
		}
		processed++
		if processed == 13 {
			// 13th PR processed returns an error because it is very, very unlucky.
			return fmt.Errorf("some munge error")
		}
		return nil
	}
	// Test normal run without errors.
	err := client.ForEachPR("k8s", "kuber", &github.PullRequestListOptions{}, false, process)
	if err != nil {
		t.Errorf("Unexpected error from ForEachPR: %v.", err)
	}
	if processed != svc.prCount {
		t.Errorf("Expected ForEachPR to process %d PRs, but %d were processed.", svc.prCount, processed)
	}

	// Test break on error.
	processed = 0
	svc.prCount = 16
	err = client.ForEachPR("k8s", "kuber", &github.PullRequestListOptions{}, false, process)
	if err == nil {
		t.Fatal("Expected error from ForEachPR after processing 13th PR, but got none.")
	}
	if processed != 13 {
		t.Errorf("Expected 13 PRs to be processed, but %d were processed.", processed)
	}

	// Test continue on error.
	processed = 0
	err = client.ForEachPR("k8s", "kuber", &github.PullRequestListOptions{}, true, process)
	if err != nil {
		t.Fatalf("Unexpected error from ForEachPR with continue-on-error enabled: %v", err)
	}
	if processed != 16 {
		t.Errorf("Expected 16 PRs to be processed, but %d were processed.", processed)
	}
}
