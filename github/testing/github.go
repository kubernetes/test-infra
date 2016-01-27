/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

package testing

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/golang/glog"
	"github.com/google/go-github/github"
)

var (
	_ = glog.Errorf
)

func stringPtr(val string) *string { return &val }
func intPtr(val int) *int          { return &val }
func boolPtr(val bool) *bool       { return &val }

func timePtr(val time.Time) *time.Time { return &val }

// PullRequest returns a filled out github.PullRequest
func PullRequest(user string, merged, mergeDetermined, mergeable bool) *github.PullRequest {
	pr := &github.PullRequest{
		Title:   stringPtr("My PR title"),
		Number:  intPtr(1),
		HTMLURL: stringPtr("PR URL"),
		Head: &github.PullRequestBranch{
			SHA: stringPtr("mysha"),
		},
		User: &github.User{
			Login:     stringPtr(user),
			AvatarURL: stringPtr("MyAvatarURL"),
		},
		Merged: boolPtr(merged),
	}
	if mergeDetermined {
		pr.Mergeable = boolPtr(mergeable)
	}
	return pr
}

// Issue returns a filled out github.Issue
func Issue(user string, number int, labels []string, isPR bool) *github.Issue {
	issue := &github.Issue{
		Title:   stringPtr("My issue title"),
		Number:  intPtr(number),
		HTMLURL: stringPtr("Issue URL"),
		User: &github.User{
			Login:     stringPtr(user),
			AvatarURL: stringPtr("MyAvatarURL"),
		},
	}
	if isPR {
		issue.PullRequestLinks = &github.PullRequestLinks{}
	}
	// putting it in a map means ordering is non-deterministic
	lmap := map[int]github.Label{}
	for i, label := range labels {
		l := github.Label{
			Name: stringPtr(label),
		}
		lmap[i] = l
	}
	for _, l := range lmap {
		issue.Labels = append(issue.Labels, l)
	}
	return issue
}

// LabelTime is a struct which can be used to call Events()
// It expresses what label the event should be about and what time
// the event took place.
type LabelTime struct {
	User  string
	Label string
	Time  int64
}

// Events returns a slice of github.IssueEvent where the specified labels were
// applied at the specified times
func Events(labels []LabelTime) []github.IssueEvent {
	// putting it in a map means ordering is non-deterministic
	eMap := map[int]github.IssueEvent{}
	for i, l := range labels {
		event := github.IssueEvent{
			Event: stringPtr("labeled"),
			Label: &github.Label{
				Name: stringPtr(l.Label),
			},
			CreatedAt: timePtr(time.Unix(l.Time, 0)),
			Actor: &github.User{
				Login: stringPtr(l.User),
			},
		}
		eMap[i] = event
	}
	out := []github.IssueEvent{}
	for _, e := range eMap {
		out = append(out, e)
	}
	return out
}

// Commit returns a filled out github.Commit which happened at time.Unix(t, 0)
func Commit(sha string, t int64) *github.Commit {
	return &github.Commit{
		SHA: stringPtr(sha),
		Committer: &github.CommitAuthor{
			Date: timePtr(time.Unix(t, 0)),
		},
	}
}

// Commits returns an array of github.RepositoryCommits. The first commit
// will have happened at time `time`, the next commit `time + 1`, etc
func Commits(num int, time int64) []github.RepositoryCommit {
	// putting it in a map means ordering is non-deterministic
	cMap := map[int]github.RepositoryCommit{}
	for i := 0; i < num; i++ {
		sha := fmt.Sprintf("mysha%d", i)
		t := time + int64(i)
		commit := github.RepositoryCommit{
			SHA:    stringPtr(sha),
			Commit: Commit(sha, t),
		}
		cMap[i] = commit
	}
	out := []github.RepositoryCommit{}
	for _, c := range cMap {
		out = append(out, c)
	}
	return out
}

func updateStatusState(status *github.CombinedStatus) *github.CombinedStatus {
	prioMap := map[string]int{
		"pending": 4,
		"error":   3,
		"failure": 2,
		"success": 1,
		"":        0,
	}

	backMap := map[int]string{
		4: "pending",
		3: "error",
		2: "failure",
		1: "success",
		0: "",
	}

	sint := 1
	for _, s := range status.Statuses {
		newSint := prioMap[*s.State]
		if newSint > sint {
			sint = newSint
		}
	}
	status.State = stringPtr(backMap[sint])
	return status
}

func fillMap(sMap map[int]github.RepoStatus, contexts []string, state string) {
	for _, context := range contexts {
		s := github.RepoStatus{
			Context:   stringPtr(context),
			State:     stringPtr(state),
			UpdatedAt: timePtr(time.Unix(0, 0)),
			CreatedAt: timePtr(time.Unix(0, 0)),
		}
		sMap[len(sMap)] = s
	}
}

// Status returns a github.CombinedStatus
func Status(sha string, success []string, fail []string, pending []string, errored []string) *github.CombinedStatus {
	// putting it in a map means ordering is non-deterministic
	sMap := map[int]github.RepoStatus{}

	fillMap(sMap, success, "success")
	fillMap(sMap, fail, "failure")
	fillMap(sMap, pending, "pending")
	fillMap(sMap, errored, "error")

	out := &github.CombinedStatus{
		SHA: stringPtr(sha),
	}
	for _, s := range sMap {
		out.Statuses = append(out.Statuses, s)
	}
	return updateStatusState(out)
}

// ServeIssue is a helper to load additional issues into the test server
func ServeIssue(t *testing.T, mux *http.ServeMux, issue *github.Issue) {
	issueNum := *issue.Number
	path := fmt.Sprintf("/repos/o/r/issues/%d", issueNum)
	setMux(t, mux, path, issue)
}

func setMux(t *testing.T, mux *http.ServeMux, path string, thing interface{}) {
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		var data []byte
		var err error

		switch thing := thing.(type) {
		default:
			t.Errorf("Unexpected object type in SetMux: %v", thing)
		case *github.Issue:
			data, err = json.Marshal(thing)
		case *github.PullRequest:
			data, err = json.Marshal(thing)
		case []github.IssueEvent:
			data, err = json.Marshal(thing)
		case []github.RepositoryCommit:
			data, err = json.Marshal(thing)
		case github.RepositoryCommit:
			data, err = json.Marshal(thing)
		case *github.CombinedStatus:
			data, err = json.Marshal(thing)
		case []github.User:
			data, err = json.Marshal(thing)
		}
		if err != nil {
			t.Errorf("%v", err)
		}
		if r.Method != "GET" {
			t.Errorf("Unexpected method: expected: GET got: %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write(data)
	})
}

// InitServer will return a github.Client which will talk to httptest.Server,
// to retrieve information from the http.ServeMux. If an issue, pr, events, or
// commits are supplied it will repond with those on o/r/
func InitServer(t *testing.T, issue *github.Issue, pr *github.PullRequest, events []github.IssueEvent, commits []github.RepositoryCommit, status *github.CombinedStatus) (*github.Client, *httptest.Server, *http.ServeMux) {
	// test server
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	// github client configured to use test server
	client := github.NewClient(nil)
	url, _ := url.Parse(server.URL)
	client.BaseURL = url
	client.UploadURL = url

	issueNum := 1
	if issue != nil {
		issueNum = *issue.Number
	} else if pr != nil {
		issueNum = *pr.Number
	}

	sha := "mysha"
	if pr != nil {
		sha = *pr.Head.SHA
	}

	if issue != nil {
		path := fmt.Sprintf("/repos/o/r/issues/%d", issueNum)
		setMux(t, mux, path, issue)
	}
	if pr != nil {
		path := fmt.Sprintf("/repos/o/r/pulls/%d", issueNum)
		setMux(t, mux, path, pr)
	}
	if events != nil {
		path := fmt.Sprintf("/repos/o/r/issues/%d/events", issueNum)
		setMux(t, mux, path, events)
	}
	if commits != nil {
		path := fmt.Sprintf("/repos/o/r/pulls/%d/commits", issueNum)
		setMux(t, mux, path, commits)
		for _, c := range commits {
			path := fmt.Sprintf("/repos/o/r/commits/%s", *c.SHA)
			setMux(t, mux, path, c)
		}
	}
	if status != nil {
		path := fmt.Sprintf("/repos/o/r/commits/%s/status", sha)
		setMux(t, mux, path, status)
	}
	path := "/repos/o/r/collaborators"
	setMux(t, mux, path, []github.User{})
	return client, server, mux
}
