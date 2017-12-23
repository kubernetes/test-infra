/*
Copyright 2014 The Kubernetes Authors.

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
	"bytes"
	"fmt"
	"net/http"
	"os"
	"sort"
	"time"

	"github.com/google/go-github/github"
	flag "github.com/spf13/pflag"
	"golang.org/x/oauth2"
)

var (
	base          string
	last          int
	current       int
	token         string
	relnoteFilter bool
)

type byMerged []*github.PullRequest

func (a byMerged) Len() int           { return len(a) }
func (a byMerged) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byMerged) Less(i, j int) bool { return a[i].MergedAt.Before(*a[j].MergedAt) }

func init() {
	flag.IntVar(&last, "last-release-pr", 0, "The PR number of the last versioned release.")
	flag.IntVar(&current, "current-release-pr", 0, "The PR number of the current versioned release.")
	flag.StringVar(&token, "api-token", "", "Github api token for rate limiting. Background: https://developer.github.com/v3/#rate-limiting and create a token: https://github.com/settings/tokens")
	flag.StringVar(&base, "base", "master", "The base branch name for PRs to look for.")
	flag.BoolVar(&relnoteFilter, "relnote-filter", true, "Whether to filter PRs by the release-note label.")
}

func usage() {
	fmt.Printf(`usage: release-notes --last-release-pr=<number> --current-release-pr=<number>
                     --api-token=<token> [--base=<branch-name>]
`)
}

func main() {
	flag.Parse()
	if last == 0 || current == 0 || token == "" {
		usage()
		os.Exit(1)
	}

	var tc *http.Client
	if len(token) > 0 {
		tc = oauth2.NewClient(
			oauth2.NoContext,
			oauth2.StaticTokenSource(
				&oauth2.Token{AccessToken: token}),
		)
	}
	client := github.NewClient(tc)

	opts := github.PullRequestListOptions{
		State:     "closed",
		Base:      base,
		Sort:      "updated",
		Direction: "desc",
		ListOptions: github.ListOptions{
			Page:    0,
			PerPage: 100,
		},
	}

	done := false
	prs := []*github.PullRequest{}
	var lastVersionMerged *time.Time
	var currentVersionMerged *time.Time
	for !done {
		opts.Page++
		fmt.Printf("Fetching PR list page %2d\n", opts.Page)
		results, _, err := client.PullRequests.List("kubernetes", "kubernetes", &opts)
		if err != nil {
			fmt.Printf("Error contacting github: %v", err)
			os.Exit(1)
		}
		unmerged := 0
		merged := 0
		if len(results) == 0 {
			done = true
			break
		}
		for ix := range results {
			result := &results[ix]
			// Skip Closed but not Merged PRs
			if result.MergedAt == nil {
				unmerged++
				continue
			}
			if *result.Number == last {
				lastVersionMerged = result.MergedAt
				fmt.Printf(" ... found last PR %d.\n", last)
				break
			}
			if lastVersionMerged != nil && lastVersionMerged.After(*result.UpdatedAt) {
				done = true
				break
			}
			if *result.Number == current {
				currentVersionMerged = result.MergedAt
				fmt.Printf(" ... found current PR %d.\n", current)
			}
			prs = append(prs, result)
			merged++
		}
		fmt.Printf(" ... %d merged PRs, %d unmerged PRs.\n", merged, unmerged)
	}
	fmt.Printf("Looking at each PR to see if it is between #%d and #%d and has the release-note label\n", last, current)
	sort.Sort(byMerged(prs))
	buffer := &bytes.Buffer{}
	for _, pr := range prs {
		if lastVersionMerged.Before(*pr.MergedAt) && (pr.MergedAt.Before(*currentVersionMerged) || (*pr.Number == current)) {
			if !relnoteFilter {
				fmt.Fprintf(buffer, "   * %s (#%d, @%s)\n", *pr.Title, *pr.Number, *pr.User.Login)
			} else {
				// Check to see if it has the release-note label.
				fmt.Printf(".")
				labels, _, err := client.Issues.ListLabelsByIssue("kubernetes", "kubernetes", *pr.Number, &github.ListOptions{})
				// Sleep for 5 seconds to avoid irritating the API rate limiter.
				time.Sleep(5 * time.Second)
				if err != nil {
					fmt.Printf("Error contacting github: %v", err)
					os.Exit(1)
				}
				for _, label := range labels {
					if *label.Name == "release-note" {
						fmt.Fprintf(buffer, "   * %s (#%d, @%s)\n", *pr.Title, *pr.Number, *pr.User.Login)
					}
				}
			}
		}
	}
	fmt.Println()
	fmt.Printf("Release notes for PRs between #%d and #%d against branch %q:\n\n", last, current, base)
	fmt.Printf("%s", buffer.Bytes())
}
