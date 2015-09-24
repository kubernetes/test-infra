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

package pulls

import (
	"fmt"
	"net/http"
	"time"

	"k8s.io/kubernetes/pkg/util/sets"

	github_util "k8s.io/contrib/mungegithub/github"
	"k8s.io/contrib/mungegithub/pulls/e2e"

	"github.com/golang/glog"
	github_api "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

const (
	needsOKToMergeLabel = "needs-ok-to-merge"
)

var (
	_ = fmt.Print
)

// SubmitQueue will merge PR which meet a set of requirements.
//  PR must have LGTM after the last commit
//  PR must have passed all github CI checks
//  if user not in whitelist PR must have "ok-to-merge"
//  The google internal jenkins instance must be passing the JenkinsJobs e2e tests
type SubmitQueue struct {
	JenkinsJobs            []string
	JenkinsHost            string
	Whitelist              string
	RequiredStatusContexts []string
	WhitelistOverride      string
	Committers             string
	Address                string
	DontRequireE2ELabel    string
	E2EStatusContext       string
	WWWRoot                string

	// additionalUserWhitelist are non-committer users believed safe
	additionalUserWhitelist *sets.String
	// CommitterList are static here in case they can't be gotten dynamically;
	// they do not need to be whitelisted.
	committerList *sets.String
	// userWhitelist is the combination of committers and additional which
	// we actully use
	userWhitelist *sets.String

	e2e *e2e.E2ETester
}

func init() {
	RegisterMungerOrDie(&SubmitQueue{})
}

// Name is the name usable in --pr-mungers
func (sq SubmitQueue) Name() string { return "submit-queue" }

// Initialize will initialize the munger
func (sq *SubmitQueue) Initialize(config *github_util.Config) error {
	if len(sq.JenkinsHost) == 0 {
		glog.Fatalf("--jenkins-host is required.")
	}

	e2e := &e2e.E2ETester{
		JenkinsJobs: sq.JenkinsJobs,
		JenkinsHost: sq.JenkinsHost,
		BuildStatus: map[string]string{},
	}
	sq.e2e = e2e
	if len(sq.Address) > 0 {
		if len(sq.WWWRoot) > 0 {
			http.Handle("/", http.FileServer(http.Dir(sq.WWWRoot)))
		}
		http.Handle("/api", e2e)
		go http.ListenAndServe(sq.Address, nil)
	}
	sq.RefreshWhitelist(config)
	return nil
}

// AddFlags will add any request flags to the cobra `cmd`
func (sq *SubmitQueue) AddFlags(cmd *cobra.Command, config *github_util.Config) {
	cmd.Flags().StringSliceVar(&sq.JenkinsJobs, "jenkins-jobs", []string{"kubernetes-e2e-gce", "kubernetes-e2e-gke-ci", "kubernetes-build", "kubernetes-e2e-gce-parallel", "kubernetes-e2e-gce-autoscaling", "kubernetes-e2e-gce-reboot", "kubernetes-e2e-gce-scalability"}, "Comma separated list of jobs in Jenkins to use for stability testing")
	cmd.Flags().StringVar(&sq.JenkinsHost, "jenkins-host", "", "The URL for the jenkins job to watch")
	cmd.Flags().StringSliceVar(&sq.RequiredStatusContexts, "required-contexts", []string{"cla/google", "Shippable", "continuous-integration/travis-ci/pr"}, "Comma separate list of status contexts required for a PR to be considered ok to merge")
	cmd.Flags().StringVar(&sq.Address, "address", ":8080", "The address to listen on for HTTP Status")
	cmd.Flags().StringVar(&sq.DontRequireE2ELabel, "dont-require-e2e-label", "e2e-not-required", "If non-empty, a PR with this label will be merged automatically without looking at e2e results")
	cmd.Flags().StringVar(&sq.E2EStatusContext, "e2e-status-context", "Jenkins GCE e2e", "The name of the github status context for the e2e PR Builder")
	cmd.Flags().StringVar(&sq.WWWRoot, "www", "", "Path to static web files to serve from the webserver")
	sq.addWhitelistCommand(cmd, config)
}

func (sq *SubmitQueue) validateLGTMAfterPush(config *github_util.Config, pr *github_api.PullRequest, lastModifiedTime *time.Time) (bool, error) {
	var lgtmTime *time.Time
	events, err := config.GetAllEventsForPR(*pr.Number)
	if err != nil {
		return false, err
	}
	for ix := range events {
		event := &events[ix]
		if *event.Event == "labeled" && *event.Label.Name == "lgtm" {
			if lgtmTime == nil || event.CreatedAt.After(*lgtmTime) {
				lgtmTime = event.CreatedAt
			}
		}
	}
	if lgtmTime == nil {
		return false, fmt.Errorf("couldn't find time for LGTM label, this shouldn't happen, skipping PR: %d", *pr.Number)
	}
	return lastModifiedTime.Before(*lgtmTime), nil
}

// MungePullRequest is the workhorse the will actually make updates to the PR
func (sq *SubmitQueue) MungePullRequest(config *github_util.Config, pr *github_api.PullRequest, issue *github_api.Issue, commits []github_api.RepositoryCommit, events []github_api.IssueEvent) {
	e2e := sq.e2e
	userSet := sq.userWhitelist

	// Don't even think about submitting without LGTM and CLA
	if !github_util.HasLabels(issue.Labels, []string{"lgtm", "cla: yes"}) {
		return
	}

	if !github_util.HasLabel(issue.Labels, sq.WhitelistOverride) && !userSet.Has(*pr.User.Login) {
		glog.V(4).Infof("Dropping %d since %s isn't in whitelist and %s isn't present", *pr.Number, *pr.User.Login, sq.WhitelistOverride)
		if !github_util.HasLabel(issue.Labels, needsOKToMergeLabel) {
			config.AddLabels(*pr.Number, []string{needsOKToMergeLabel})
			body := "The author of this PR is not in the whitelist for merge, can one of the admins add the 'ok-to-merge' label?"
			config.WriteComment(*pr.Number, body)
		}
		return
	}

	// Tidy up the issue list.
	if github_util.HasLabel(issue.Labels, needsOKToMergeLabel) {
		config.RemoveLabel(*pr.Number, needsOKToMergeLabel)
	}

	lastModifiedTime, err := config.LastModifiedTime(*pr.Number)
	if err != nil {
		glog.Errorf("Failed to get last modified time, skipping PR: %d", *pr.Number)
		return
	}
	if ok, err := sq.validateLGTMAfterPush(config, pr, lastModifiedTime); err != nil {
		glog.Errorf("Error validating LGTM: %v, Skipping: %d", err, *pr.Number)
		return
	} else if !ok {
		glog.Errorf("PR pushed after LGTM, attempting to remove LGTM and skipping")
		staleLGTMBody := "LGTM was before last commit, removing LGTM"
		config.WriteComment(*pr.Number, staleLGTMBody)
		config.RemoveLabel(*pr.Number, "lgtm")
		return
	}

	if mergeable, err := config.IsPRMergeable(pr); err != nil {
		glog.V(2).Infof("Skipping %d - unable to determine mergeability", *pr.Number)
	} else if !mergeable {
		glog.V(2).Infof("Skipping %d - not mergable", *pr.Number)
	}

	// Validate the status information for this PR
	contexts := sq.RequiredStatusContexts
	if len(sq.DontRequireE2ELabel) == 0 || !github_util.HasLabel(issue.Labels, sq.DontRequireE2ELabel) {
		contexts = append(contexts, sq.E2EStatusContext)
	}
	if ok := config.IsStatusSuccess(pr, contexts); !ok {
		glog.Errorf("PR# %d CI status is not success", *pr.Number)
		return
	}

	if !e2e.Stable() {
		glog.Errorf("Error jenkins e2e tests not stable: %v", err)
		return
	}

	// if there is a 'e2e-not-required' label, just merge it.
	if len(sq.DontRequireE2ELabel) == 0 || !github_util.HasLabel(issue.Labels, sq.DontRequireE2ELabel) {
		config.MergePR(pr, "submit-queue")
		return
	}

	body := "@k8s-bot test this [submit-queue is verifying that this PR is safe to merge]"
	if err := config.WriteComment(*pr.Number, body); err != nil {
		return
	}

	// Wait for the build to start
	_ = config.WaitForPending(pr)
	_ = config.WaitForNotPending(pr)

	// Wait for the status to go back to 'success'
	if ok := config.IsStatusSuccess(pr, contexts); !ok {
		glog.Errorf("Status after build is not 'success', skipping PR %d", *pr.Number)
		return
	}

	config.MergePR(pr, "submit-queue")
	return
}
