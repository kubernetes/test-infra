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

package gatherer

import (
	"encoding/json"
	"github.com/sirupsen/logrus"
	"github.com/tomnomnom/linkheader"
	"io"
	"k8s.io/test-infra/ghproxy/ghcache"
	"net/http"
)

// JSONPresence detects if a particular key is present without actually parsing any of the data
type JSONPresence struct {
	present bool
}

func (j *JSONPresence) UnmarshalJSON([]byte) error {
	j.present = true
	return nil
}

type GitHubIssueScraper struct {
	githubRoundTripper http.RoundTripper
	RepoInfix          string
}

type GitHubIssue struct {
	IssueId     int          `json:"number"`
	Title       string       `json:"title"`
	CommentsUrl string       `json:"comments_url"`
	NumComments int          `json:"comments"`
	Comments    []string     `json:"-"` //ignore
	PullRequest JSONPresence `json:"pull_request"`
	Body        string       `json:"body"`
}

func (g *GitHubIssueScraper) GetIssues() []GitHubIssue {

	thisPage := "https://api.github.com/repos/" + g.RepoInfix + "/issues"
	issues := make([]GitHubIssue, 0)

	for {
		resp := g.askGitHub(thisPage)

		links := linkheader.Parse(resp.Header.Get("Link"))
		issues = append(issues, g.parseIssuePage(resp.Body)...)

		var nextPage string
		for _, link := range links {
			if link.Rel == "next" {
				nextPage = link.URL
			}
		}

		logrus.Infof("This Page: %s, Next Page: %s", thisPage, nextPage)
		if nextPage == "" {
			break
		}
		thisPage = nextPage
	}

	return issues
}

func NewIssueScraper(ownerAndRepo string) GitHubIssueScraper {
	return GitHubIssueScraper{
		RepoInfix: ownerAndRepo,
	}
}

func (g *GitHubIssueScraper) initializeCache() {
	rt := &http.Transport{}
	g.githubRoundTripper = ghcache.NewMemCache(rt, 3)
}

func (g *GitHubIssueScraper) parseCommentsPage(body io.Reader) []string {
	dec := json.NewDecoder(body)

	comments := make([]string, 0)

	// read open bracket
	dec.Token()
	for dec.More() {
		var m struct {
			Body string `json:"body"`
		}
		err := dec.Decode(&m)
		if err != nil {
			logrus.Fatal(err)
		}

		comments = append(comments, m.Body)
	}

	return comments
}

func (g *GitHubIssueScraper) parseIssuePage(body io.Reader) []GitHubIssue {
	dec := json.NewDecoder(body)

	// read open bracket
	dec.Token()

	issues := make([]GitHubIssue, 0)

	for dec.More() {
		var m GitHubIssue
		err := dec.Decode(&m)
		if err != nil {
			logrus.Fatal(err)
		}

		// don't include pull requests
		if !m.PullRequest.present {
			// get & populate comments, if any
			if m.NumComments != 0 {
				comRP := g.askGitHub(m.CommentsUrl)
				comments := g.parseCommentsPage(comRP.Body)
				m.Comments = comments
			}
			issues = append(issues, m)
		}
	}

	return issues
}

func (g *GitHubIssueScraper) askGitHub(url string) *http.Response {
	if g.githubRoundTripper == nil {
		g.initializeCache()
	}

	rq, _ := http.NewRequest(http.MethodGet, url, nil)
	rp, err := g.githubRoundTripper.RoundTrip(rq)
	if err != nil {
		logrus.Errorf("Error in GitHub Call: %e", err)
	}
	logrus.Infof("Call to %s: %s", url, rp.Status)

	return rp
}
