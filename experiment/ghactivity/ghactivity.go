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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/google/go-github/v24/github"
	"golang.org/x/oauth2"
)

var githubToken = flag.String("github-token", "", "GitHub API OAUTH token")
var author = flag.String("author", "", "Report issues/PRs for this GitHub user")
var start = flag.String("start", "", "Start month, e.g. 'Feb 2018'")
var end = flag.String("end", "", "End month, e.g. 'Mar 2019', defaults to today")

type prDetails struct {
	ReviewComments int
	Commits        int
	LineAdditions  int
	LineDeletions  int
	ChangedFiles   int
	Merged         bool
}

type issueDetails struct {
	Type        string
	Org         string
	Repo        string
	Number      int
	Title       string
	CreatedDate time.Time
	ClosedDate  time.Time
	UpdatedDate time.Time
	Comments    int
	State       string
	PrDetails   *prDetails
}

var repoRE = regexp.MustCompile(".*/repos/([a-zA-Z0-9_-]+)/([a-zA-Z0-9_-]+)")

func getPRDetails(client *github.Client, org, repo string, number int) (*prDetails, error) {
	pr, _, err := client.PullRequests.Get(context.Background(), org, repo, number)
	if err != nil {
		return nil, err
	}
	pd := prDetails{
		ReviewComments: pr.GetReviewComments(),
		Commits:        pr.GetCommits(),
		LineAdditions:  pr.GetAdditions(),
		LineDeletions:  pr.GetDeletions(),
		ChangedFiles:   pr.GetChangedFiles(),
		Merged:         pr.GetMerged(),
	}
	return &pd, nil
}

func issuesForMonth(ctx context.Context, client *github.Client, author string, year int, month time.Month) ([]*issueDetails, int, int, error) {
	startDate := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	endDate := startDate.AddDate(0, 1, -1)
	query := fmt.Sprintf("author:\"%s\" created:%s..%s", author, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	opts := &github.SearchOptions{
		Sort:  "created",
		Order: "asc",
	}
	var ids []*issueDetails
	totalIssues := 0
	totalPRs := 0
	// TODO: maybe use pkg/ghclient for retry and rate limiting logic?
	for {
		issues, resp, err := client.Search.Issues(ctx, query, opts)
		if _, ok := err.(*github.RateLimitError); ok {
			log.Println("hit rate limit")
		}
		if err != nil {
			return nil, -1, -1, err
		}

		for _, issue := range issues.Issues {
			org := ""
			repo := ""
			if m := repoRE.FindStringSubmatch(issue.GetRepositoryURL()); m != nil {
				org = m[1]
				repo = m[2]
			}
			id := issueDetails{
				Type:        "issue",
				Org:         org,
				Repo:        repo,
				Number:      issue.GetNumber(),
				Title:       issue.GetTitle(),
				CreatedDate: issue.GetCreatedAt(),
				ClosedDate:  issue.GetClosedAt(),
				UpdatedDate: issue.GetUpdatedAt(),
				Comments:    issue.GetComments(),
				State:       issue.GetState(),
			}
			if issue.IsPullRequest() {
				totalPRs++
				id.Type = "PR"
				if pd, err := getPRDetails(client, id.Org, id.Repo, id.Number); err == nil {
					id.PrDetails = pd
					if pd.Merged {
						id.State = "merged"
					}
				}
			} else {
				totalIssues++
			}
			ids = append(ids, &id)
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return ids, totalIssues, totalPRs, nil
}

func main() {
	flag.Parse()
	if *author == "" {
		log.Fatalf("--author must be provided")
	}
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: *githubToken},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	client := github.NewClient(tc)

	startDate, err := time.Parse("Jan 2006", *start)
	if err != nil {
		log.Fatalf("failed parsing --start: %v", err)
	}
	endDate := time.Now()
	if *end != "" {
		endDate, err = time.Parse("Jan 2006", *end)
		if err != nil {
			log.Fatalf("failed parsing --end: %v", err)
		}
	}

	fmt.Printf("GitHub activity report for %s\n\n", *author)
	month := startDate
	for !month.After(endDate) && !month.After(time.Now()) {
		fmt.Printf("%s\n==============\n", month.Format("January 2006"))
		ids, totalIssues, totalPRs, err := issuesForMonth(context.Background(), client, *author, month.Year(), month.Month())
		if err != nil {
			log.Fatalf("encountered error: %v", err)
		}
		fmt.Printf("Created %d Issues and %d PRs\n\n", totalIssues, totalPRs)
		for _, id := range ids {
			fmt.Printf("[%s] %s (%s/%s#%d)\n", strings.Title(id.Type), id.Title, id.Org, id.Repo, id.Number)
			fmt.Printf("  Created %s\n", id.CreatedDate.Format("2 Jan 2006"))
			pd := id.PrDetails
			if pd != nil {
				fmt.Printf("  %d comments, %d review comments\n", id.Comments, pd.ReviewComments)
				fmt.Printf("  %d commits, +%d/-%d lines, %d changed files\n",
					pd.Commits, pd.LineAdditions, pd.LineDeletions, pd.ChangedFiles)
			} else {
				fmt.Printf("  %d comments\n", id.Comments)
			}
			if id.State == "merged" || id.State == "closed" {
				fmt.Printf("  %s %s\n", strings.ToUpper(id.State), id.ClosedDate.Format("2 Jan 2006"))
			} else {
				fmt.Printf("  %s (last update %s)\n", strings.ToUpper(id.State), id.UpdatedDate.Format("2 Jan 2006"))
			}
		}
		month = month.AddDate(0, 1, 0)
		fmt.Printf("\n")
	}
}
