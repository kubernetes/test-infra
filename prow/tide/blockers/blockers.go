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

package blockers

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	githubql "github.com/shurcooL/githubv4"
	"github.com/sirupsen/logrus"
)

var (
	branchRE = regexp.MustCompile(`(?im)\bbranch:[^\w-]*([\w-./]+)\b`)
)

type githubClient interface {
	QueryWithGitHubAppsSupport(ctx context.Context, q interface{}, vars map[string]interface{}, org string) error
}

// Blocker specifies an issue number that should block tide from merging.
type Blocker struct {
	Number     int
	Title, URL string
	// TODO: time blocked? (when blocker label was added)
}

type OrgRepo struct {
	Org, Repo string
}

type OrgRepoBranch struct {
	Org, Repo, Branch string
}

// Blockers holds maps of issues that are blocking various repos/branches.
type Blockers struct {
	Repo   map[OrgRepo][]Blocker       `json:"repo,omitempty"`
	Branch map[OrgRepoBranch][]Blocker `json:"branch,omitempty"`
}

// GetApplicable returns the subset of blockers applicable to the specified branch.
func (b Blockers) GetApplicable(org, repo, branch string) []Blocker {
	var res []Blocker
	res = append(res, b.Repo[OrgRepo{Org: org, Repo: repo}]...)
	res = append(res, b.Branch[OrgRepoBranch{Org: org, Repo: repo, Branch: branch}]...)

	sort.Slice(res, func(i, j int) bool {
		return res[i].Number < res[j].Number
	})
	return res
}

// FindAll finds issues with label in the specified orgs/repos that should block tide.
func FindAll(ghc githubClient, log *logrus.Entry, label, orgRepoTokens string) (Blockers, error) {
	issues, err := search(
		context.Background(),
		ghc,
		log,
		blockerQuery(label, orgRepoTokens),
	)
	if err != nil {
		return Blockers{}, fmt.Errorf("error searching for blocker issues: %v", err)
	}

	return fromIssues(issues, log), nil
}

func fromIssues(issues []Issue, log *logrus.Entry) Blockers {
	log.Debugf("Finding blockers from %d issues.", len(issues))
	res := Blockers{Repo: make(map[OrgRepo][]Blocker), Branch: make(map[OrgRepoBranch][]Blocker)}
	for _, issue := range issues {
		logger := log.WithFields(logrus.Fields{"org": issue.Repository.Owner.Login, "repo": issue.Repository.Name, "issue": issue.Number})
		strippedTitle := branchRE.ReplaceAllLiteralString(string(issue.Title), "")
		block := Blocker{
			Number: int(issue.Number),
			Title:  strippedTitle,
			URL:    string(issue.URL),
		}
		if branches := parseBranches(string(issue.Title)); len(branches) > 0 {
			for _, branch := range branches {
				key := OrgRepoBranch{
					Org:    string(issue.Repository.Owner.Login),
					Repo:   string(issue.Repository.Name),
					Branch: branch,
				}
				logger.WithField("branch", branch).Debug("Blocking merges to branch via issue.")
				res.Branch[key] = append(res.Branch[key], block)
			}
		} else {
			key := OrgRepo{
				Org:  string(issue.Repository.Owner.Login),
				Repo: string(issue.Repository.Name),
			}
			logger.Debug("Blocking merges to all branches via issue.")
			res.Repo[key] = append(res.Repo[key], block)
		}
	}
	return res
}

func blockerQuery(label, orgRepoTokens string) string {
	tokens := []string{
		"is:issue",
		"state:open",
		fmt.Sprintf("label:\"%s\"", label),
		orgRepoTokens,
	}
	return strings.Join(tokens, " ")
}

func parseBranches(str string) []string {
	var res []string
	for _, match := range branchRE.FindAllStringSubmatch(str, -1) {
		res = append(res, match[1])
	}
	return res
}

func search(ctx context.Context, ghc githubClient, log *logrus.Entry, q string) ([]Issue, error) {
	requestStart := time.Now()
	var ret []Issue
	vars := map[string]interface{}{
		"query":        githubql.String(q),
		"searchCursor": (*githubql.String)(nil),
	}
	var totalCost int
	var remaining int
	for {
		sq := searchQuery{}
		// TODO alvaroaleman: Add github apps support
		if err := ghc.QueryWithGitHubAppsSupport(ctx, &sq, vars, ""); err != nil {
			return nil, err
		}
		totalCost += int(sq.RateLimit.Cost)
		remaining = int(sq.RateLimit.Remaining)
		for _, n := range sq.Search.Nodes {
			ret = append(ret, n.Issue)
		}
		if !sq.Search.PageInfo.HasNextPage {
			break
		}
		vars["searchCursor"] = githubql.NewString(sq.Search.PageInfo.EndCursor)
	}
	log.WithField(
		"duration", time.Since(requestStart).String(),
	).Debugf("Search for blocker query \"%s\" cost %d point(s). %d remaining.", q, totalCost, remaining)
	return ret, nil
}

// Issue holds graphql response data about issues
type Issue struct {
	Number     githubql.Int
	Title      githubql.String
	URL        githubql.String
	Repository struct {
		Name  githubql.String
		Owner struct {
			Login githubql.String
		}
	}
}

type searchQuery struct {
	RateLimit struct {
		Cost      githubql.Int
		Remaining githubql.Int
	}
	Search struct {
		PageInfo struct {
			HasNextPage githubql.Boolean
			EndCursor   githubql.String
		}
		Nodes []struct {
			Issue Issue `graphql:"... on Issue"`
		}
	} `graphql:"search(type: ISSUE, first: 100, after: $searchCursor, query: $query)"`
}
