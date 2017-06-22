/*
Copyright 2016 The Kubernetes Authors.

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

package trigger

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
)

func boolPtr(val bool) *bool { return &val }

func TestRebase(t *testing.T) {
	// Most important is `Mergeable` flag on PR, here are initial values:
	// nil means that github didn't set that flag, so we should not add/remove label then
	// true - we need to remove label (needsRebase) if it is present
	// false - we need to set label (needsRebase) if it is not there yet
	// nil, t, f, nil, t, f
	prs := []github.PullRequest{
		{Number: 1, State: "open", Merged: false, Mergeable: nil},
		{Number: 2, State: "open", Merged: false, Mergeable: boolPtr(true)},
		{Number: 3, State: "open", Merged: false, Mergeable: boolPtr(false)},
		{Number: 4, State: "open", Merged: false, Mergeable: nil},
		{Number: 5, State: "open", Merged: false, Mergeable: boolPtr(true)},
		{Number: 6, State: "open", Merged: false, Mergeable: boolPtr(false)},
	}
	// And here we have 6 issues: first 3 have label (needsRebase) set, last 3 don't
	// FIXME: Labels are set on Issue not on the PR - is this OK?
	issues := []github.Issue{
		{Number: 11, Labels: []github.Label{{Name: needsRebaseLabel}}, PullRequest: &prs[0]},
		{Number: 12, Labels: []github.Label{{Name: needsRebaseLabel}}, PullRequest: &prs[1]},
		{Number: 13, Labels: []github.Label{{Name: needsRebaseLabel}}, PullRequest: &prs[2]},
		{Number: 14, Labels: []github.Label{}, PullRequest: &prs[3]},
		{Number: 15, Labels: []github.Label{}, PullRequest: &prs[4]},
		{Number: 16, Labels: []github.Label{}, PullRequest: &prs[5]},
	}

	// Only 4th PullRequestEvent can actually change anything, others should not go to "needs-rebase" label manipulation code
	// 4th PR is real merge to master branch: it is (action=closed, merged=true, base.ref=master)
	// Here only last line (4th PREvent) makes changes.
	var testcases = []github.PullRequestEvent{
		{Action: "edited", Number: 21, PullRequest: github.PullRequest{Number: 21, Merged: false, Base: github.PullRequestBranch{Ref: "example"}}},
		{Action: "closed", Number: 22, PullRequest: github.PullRequest{Number: 22, Merged: false, Base: github.PullRequestBranch{Ref: "example"}}},
		{Action: "closed", Number: 23, PullRequest: github.PullRequest{Number: 23, Merged: true, Base: github.PullRequestBranch{Ref: "example"}}},
		{Action: "closed", Number: 24, PullRequest: github.PullRequest{Number: 24, Merged: true, Base: github.PullRequestBranch{Ref: "master"}}},
	}
	// For 1st PR Mergeable is nil, so we do nothing (leave label as is)
	// For 2nd PR Mergeable is true and we have a label set - so remove it (no more needs rebase) (*)
	// For 3rd PR Mergeable is false and we have label, so leave it (still needs rebase)
	// For 4th PR Mergeable is nil, so we do nothing (and there is no label there currently)
	// For 5th PR Mergeable is true and we have no label, so we do nothing
	// For 6th PR Mergeable is false and we have no label, so we add needsRebase label (*)
	// So for last operation we should add `needsRebaseLabel` to Issue #16 and remove it from Issue #12
	expectLabels := []struct {
		addedLabels   []string
		removedLabels []string
	}{
		{},
		{},
		{},
		{addedLabels: []string{"/#16:" + needsRebaseLabel}, removedLabels: []string{"/#12:" + needsRebaseLabel}},
	}

	// This test and also testing via: `./bazel-bin/prow/cmd/hook/hook --local ...` won't hit real github API
	// To test this we need to run real bot with real repo, hmac, oauth etc.
	// Consider making those tests more accurate, but then our CI will hit GH api points.
	errs := []string{}
	for index, prEvent := range testcases {
		i := index + 1
		g := &fakegithub.FakeClient{
			Issues: issues,
		}
		c := client{
			GitHubClient: g,
			Config:       &config.Config{},
			Logger:       logrus.WithField("plugin", pluginName),
		}

		// This is a special flag (that can't happen in real call) that tells handlePR to skip
		// waiting for GitHug "mergeable" status propagation on other PRs
		// In test environment those statuses are set during test cases setup and we don't want test to run for > 2 minutes
		prEvent.PullRequest.State = "skip_sleep"
		err := handlePR(c, prEvent)
		if err != nil {
			t.Fatalf("Test #%d handlePR: Didn't expect error: %s", i, err)
		}

		if len(expectLabels[index].addedLabels) > 0 {
			if !reflect.DeepEqual(expectLabels[index].addedLabels, g.LabelsAdded) {
				errs = append(errs, fmt.Sprintf("Test #%d Expected to add `%v` Labels, while added `%v`", i, expectLabels[index].addedLabels, g.LabelsAdded))
			}
		}

		if len(expectLabels[index].removedLabels) > 0 {
			if !reflect.DeepEqual(expectLabels[index].removedLabels, g.LabelsRemoved) {
				errs = append(errs, fmt.Sprintf("Test #%d Expected to remove `%v` Labels, while removed `%v`", i, expectLabels[index].removedLabels, g.LabelsRemoved))
			}
		}
	}

	if len(errs) > 0 {
		t.Fatalf(strings.Join(errs[:], "\n"))
	}
}
