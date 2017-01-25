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

// Package crier sets GitHub statuses and writes comments for test statuses.
package crier

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins"
)

const (
	commentTag  = "<!-- test report -->"
	guberPrefix = "https://k8s-gubernator.appspot.com/pr/"
)

type Report struct {
	RepoOwner string `json:"repo_owner"`
	RepoName  string `json:"repo_name"`
	Author    string `json:"author"`
	Number    int    `json:"number"`
	Commit    string `json:"commit"`

	Context      string `json:"context"`
	State        string `json:"state"`
	Description  string `json:"description"`
	RerunCommand string `json:"rerun_command"`
	URL          string `json:"url"`
}

type Server struct {
	rc     chan Report
	ghc    GitHubClient
	notify chan struct{}
}

type GitHubClient interface {
	CreateStatus(org, repo, ref string, s github.Status) error
	ListIssueComments(org, repo string, number int) ([]github.IssueComment, error)
	CreateComment(org, repo string, number int, comment string) error
	DeleteComment(org, repo string, ID int) error
}

func NewServer(ghc GitHubClient) *Server {
	return &Server{
		rc:  make(chan Report),
		ghc: ghc,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/status" {
		http.Error(w, fmt.Sprintf("Bad path: %s", r.URL.Path), http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, fmt.Sprintf("Method not POST: %s", r.Method), http.StatusMethodNotAllowed)
		return
	}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error reading request body: %v", err), http.StatusInternalServerError)
		return
	}
	var report Report
	if err := json.Unmarshal(body, &report); err != nil {
		http.Error(w, fmt.Sprintf("Error unmarshaling JSON: %v", err), http.StatusBadRequest)
		return
	}
	go func() {
		s.rc <- report
	}()
}

func (s *Server) Run() {
	go func() {
		for report := range s.rc {
			if err := s.handle(report); err != nil {
				logrus.WithFields(logrus.Fields{
					"org":  report.RepoOwner,
					"repo": report.RepoName,
					"pr":   report.Number,
				}).WithError(err).Error("Handle failed.")
			}
		}
	}()
}

// parseIssueComments returns a list of comments to delete along with a list
// of table entries for the new comment. If there are no table entries then
// don't make a new comment.
func parseIssueComments(r Report, ics []github.IssueComment) ([]int, []string) {
	var delete []int
	var previousComments []int
	var entries []string
	// First accumulate result entries and comment IDs
	for _, ic := range ics {
		if ic.User.Login != "k8s-ci-robot" {
			continue
		}
		// Old report comments started with the context. Delete them.
		// TODO(spxtr): Delete this check a few weeks after this merges.
		if strings.HasPrefix(ic.Body, r.Context) {
			delete = append(delete, ic.ID)
		}
		if !strings.Contains(ic.Body, commentTag) {
			continue
		}
		previousComments = append(previousComments, ic.ID)
		var tracking bool
		for _, line := range strings.Split(ic.Body, "\n") {
			if strings.HasPrefix(line, "---") {
				tracking = true
			} else if len(strings.TrimSpace(line)) == 0 {
				tracking = false
			} else if tracking {
				entries = append(entries, line)
			}
		}
	}
	var newEntries []string
	var createNewComment = len(previousComments) > 1
	// Next decide which entries to keep.
	for i := range entries {
		keep := true
		f1 := strings.Split(entries[i], "|")
		for j := range entries {
			if i == j {
				continue
			}
			f2 := strings.Split(entries[j], "|")
			// Use the newer results if there are multiple.
			if j > i && f2[0] == f1[0] {
				keep = false
				createNewComment = true
			}
		}
		// Use the current result if there is an old one.
		if r.Context == strings.TrimSpace(f1[0]) {
			keep = false
			createNewComment = true
		}
		if keep {
			newEntries = append(newEntries, entries[i])
		}
	}
	if r.State == github.StatusFailure {
		newEntries = append(newEntries, createEntry(r))
		createNewComment = true
	}
	if createNewComment {
		return append(delete, previousComments...), newEntries
	} else {
		return delete, nil
	}
}

func createEntry(r Report) string {
	return strings.Join([]string{
		r.Context,
		r.Commit,
		fmt.Sprintf("[link](%s)", r.URL),
		fmt.Sprintf("`%s`", r.RerunCommand),
	}, " | ")
}

func prLink(r Report) string {
	var suffix string
	if r.RepoOwner == "kubernetes" {
		if r.RepoName == "kubernetes" {
			suffix = fmt.Sprintf("%d", r.Number)
		} else {
			suffix = fmt.Sprintf("%s/%d", r.RepoName, r.Number)
		}
	} else {
		suffix = fmt.Sprintf("%s_%s/%d", r.RepoOwner, r.RepoName, r.Number)
	}
	return guberPrefix + suffix
}

func dashLink(r Report) string {
	return guberPrefix + r.Author
}

// createComment takes a report and a list of entries generated with
// createEntry and returns a nicely formatted comment.
func createComment(r Report, entries []string) string {
	lines := []string{
		fmt.Sprintf("@%s: The following test(s) **failed**:", r.Author),
		"",
		"Test name | Commit | Details | Rerun command",
		"--- | --- | --- | ---",
	}
	lines = append(lines, entries...)
	lines = append(lines, []string{
		"",
		fmt.Sprintf("[Full PR test history](%s). [Your PR dashboard](%s). Please help us cut down on flakes by linking to an [open issue](https://github.com/%s/%s/issues?q=is:issue+is:open) when you hit one in your PR.", prLink(r), dashLink(r), r.RepoOwner, r.RepoName),
		"",
		"<details>",
		"",
		plugins.AboutThisBot,
		"</details>",
		commentTag,
	}...)
	return strings.Join(lines, "\n")
}

func (s *Server) handle(r Report) error {
	defer func() {
		if s.notify != nil {
			s.notify <- struct{}{}
		}
	}()
	if err := s.ghc.CreateStatus(r.RepoOwner, r.RepoName, r.Commit, github.Status{
		State:       r.State,
		Description: r.Description,
		Context:     r.Context,
		TargetURL:   r.URL,
	}); err != nil {
		return fmt.Errorf("error setting status: %v", err)
	}
	if r.State != github.StatusSuccess && r.State != github.StatusFailure {
		return nil
	}
	ics, err := s.ghc.ListIssueComments(r.RepoOwner, r.RepoName, r.Number)
	if err != nil {
		return fmt.Errorf("error listing comments: %v", err)
	}
	deletes, entries := parseIssueComments(r, ics)
	for _, delete := range deletes {
		if err := s.ghc.DeleteComment(r.RepoOwner, r.RepoName, delete); err != nil {
			return fmt.Errorf("error deleting comment: %v", err)
		}
	}
	if len(entries) > 0 {
		if err := s.ghc.CreateComment(r.RepoOwner, r.RepoName, r.Number, createComment(r, entries)); err != nil {
			return fmt.Errorf("error creating comment: %v", err)
		}
	}
	return nil
}
