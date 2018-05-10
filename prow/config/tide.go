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

package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/github"
)

const timeFormatISO8601 = "2006-01-02T15:04:05Z"

type TideQueries []TideQuery

// Tide is config for the tide pool.
type Tide struct {
	// SyncPeriodString compiles into SyncPeriod at load time.
	SyncPeriodString string `json:"sync_period,omitempty"`
	// SyncPeriod specifies how often Tide will sync jobs with Github. Defaults to 1m.
	SyncPeriod time.Duration `json:"-"`
	// StatusUpdatePeriodString compiles into StatusUpdatePeriod at load time.
	StatusUpdatePeriodString string `json:"status_update_period,omitempty"`
	// StatusUpdatePeriod specifies how often Tide will update Github status contexts.
	// Defaults to the value of SyncPeriod.
	StatusUpdatePeriod time.Duration `json:"-"`
	// Queries must not overlap. It must be impossible for any two queries to
	// ever return the same PR.
	// TODO: This will only be possible when we allow specifying orgs. At that
	//       point, verify the above condition.
	Queries TideQueries `json:"queries,omitempty"`

	// A key/value pair of an org/repo as the key and merge method to override
	// the default method of merge. Valid options are squash, rebase, and merge.
	MergeType map[string]github.PullRequestMergeType `json:"merge_method,omitempty"`

	// URL for tide status contexts.
	// We can consider allowing this to be set separately for separate repos, or
	// allowing it to be a template.
	TargetURL string `json:"target_url,omitempty"`

	// PRStatusBaseUrl is the base URL for the PR status page.
	// This is used to link to a merge requirements overview
	// in the tide status context.
	PRStatusBaseUrl string `json:"pr_status_base_url,omitempty"`

	// MaxGoroutines is the maximum number of goroutines spawned inside the
	// controller to handle org/repo:branch pools. Defaults to 20. Needs to be a
	// positive number.
	MaxGoroutines int `json:"max_goroutines,omitempty"`
}

// MergeMethod returns the merge method to use for a repo. The default of merge is
// returned when not overridden.
func (t *Tide) MergeMethod(org, repo string) github.PullRequestMergeType {
	name := org + "/" + repo

	v, ok := t.MergeType[name]
	if !ok {
		if ov, found := t.MergeType[org]; found {
			return ov
		}

		return github.MergeMerge
	}

	return v
}

// TideQuery is turned into a GitHub search query. See the docs for details:
// https://help.github.com/articles/searching-issues-and-pull-requests/
type TideQuery struct {
	Orgs  []string `json:"orgs,omitempty"`
	Repos []string `json:"repos,omitempty"`

	ExcludedBranches []string `json:"excludedBranches,omitempty"`
	IncludedBranches []string `json:"includedBranches,omitempty"`

	Labels        []string `json:"labels,omitempty"`
	MissingLabels []string `json:"missingLabels,omitempty"`

	Milestone string `json:"milestone,omitempty"`

	ReviewApprovedRequired bool `json:"reviewApprovedRequired,omitempty"`
}

func (tq *TideQuery) Query() string {
	toks := []string{"is:pr", "state:open"}
	for _, o := range tq.Orgs {
		toks = append(toks, fmt.Sprintf("org:\"%s\"", o))
	}
	for _, r := range tq.Repos {
		toks = append(toks, fmt.Sprintf("repo:\"%s\"", r))
	}
	for _, b := range tq.ExcludedBranches {
		toks = append(toks, fmt.Sprintf("-base:\"%s\"", b))
	}
	for _, b := range tq.IncludedBranches {
		toks = append(toks, fmt.Sprintf("base:\"%s\"", b))
	}
	for _, l := range tq.Labels {
		toks = append(toks, fmt.Sprintf("label:\"%s\"", l))
	}
	for _, l := range tq.MissingLabels {
		toks = append(toks, fmt.Sprintf("-label:\"%s\"", l))
	}
	if tq.Milestone != "" {
		toks = append(toks, fmt.Sprintf("milestone:\"%s\"", tq.Milestone))
	}
	if tq.ReviewApprovedRequired {
		toks = append(toks, "review:approved")
	}
	return strings.Join(toks, " ")
}

// AllPRsSince returns all open PRs in the repos covered by the query that
// have changed since time t.
func (tqs TideQueries) AllPRsSince(t time.Time) string {
	toks := []string{"is:pr", "state:open"}

	orgs := sets.NewString()
	repos := sets.NewString()
	for i := range tqs {
		orgs.Insert(tqs[i].Orgs...)
		repos.Insert(tqs[i].Repos...)
	}
	for _, o := range orgs.List() {
		toks = append(toks, fmt.Sprintf("org:\"%s\"", o))
	}
	for _, r := range repos.List() {
		toks = append(toks, fmt.Sprintf("repo:\"%s\"", r))
	}
	// Github's GraphQL API silently fails if you provide it with an invalid time
	// string.
	// Dates before 1970 are considered invalid.
	if t.Year() >= 1970 {
		toks = append(toks, fmt.Sprintf("updated:>=%s", t.Format(timeFormatISO8601)))
	}
	return strings.Join(toks, " ")
}

// QueryMap is a mapping from ("org/repo" or "org") -> TideQueries that
// apply to that org or repo.
type QueryMap map[string]TideQueries

// QueryMap creates a QueryMap from TideQueries
func (tqs TideQueries) QueryMap() QueryMap {
	res := make(map[string]TideQueries)
	for _, tq := range tqs {
		for _, org := range tq.Orgs {
			res[org] = append(res[org], tq)
		}
		for _, repo := range tq.Repos {
			res[repo] = append(res[repo], tq)
		}
	}
	return res
}

// ForRepo returns the tide queries that apply to a repo.
func (qm QueryMap) ForRepo(org, repo string) TideQueries {
	qs := TideQueries(nil)
	qs = append(qs, qm[org]...)
	qs = append(qs, qm[fmt.Sprintf("%s/%s", org, repo)]...)
	return qs
}

func (tq *TideQuery) Validate() error {
	for o := range tq.Orgs {
		if strings.Contains(tq.Orgs[o], "/") {
			return fmt.Errorf("orgs[%d]: %q contains a '/' which is not valid", o, tq.Orgs[o])
		}
		if len(tq.Orgs[o]) == 0 {
			return fmt.Errorf("orgs[%d]: is an empty string", o)
		}
	}

	for r := range tq.Repos {
		parts := strings.Split(tq.Repos[r], "/")
		if len(parts) != 2 || len(parts[0]) == 0 || len(parts[1]) == 0 {
			return fmt.Errorf("repos[%d]: %q is not of the form \"org/repo\"", r, tq.Repos[r])
		}
		for o := range tq.Orgs {
			if tq.Orgs[o] == parts[0] {
				return fmt.Errorf("repos[%d]: %q is already included via orgs[%d]: %q", r, tq.Repos[r], o, tq.Orgs[o])
			}
		}
	}

	if invalids := sets.NewString(tq.Labels...).Intersection(sets.NewString(tq.MissingLabels...)); len(invalids) > 0 {
		return fmt.Errorf("the labels: %q are both required and forbidden", invalids.List())
	}

	// Warnings
	if len(tq.ExcludedBranches) > 0 && len(tq.IncludedBranches) > 0 {
		logrus.Warning("Smell: Both included and excluded branches are specified (excluded branches have no effect).")
	}

	return nil
}
