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

package main

// A simple binary for merging PR that match a criteria
// Usage:
//   submit-queue -token=<github-access-token> -user-whitelist=<file> --jenkins-host=http://some.host [-min-pr-number=<number>] [-dry-run] [-once]

import (
	goflag "flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"k8s.io/kubernetes/pkg/util/sets"

	github "k8s.io/contrib/mungegithub/github"

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

// SubmitQueueConfig has all of the configuration for the submit queue along with
// and embedded github.Config
type SubmitQueueConfig struct {
	github.Config
	Once                   bool
	JenkinsJobs            []string
	JenkinsHost            string
	Whitelist              string
	RequiredStatusContexts []string
	WhitelistOverride      string
	Committers             string
	PollPeriod             time.Duration
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
}

func addSubmitFlags(cmd *cobra.Command, config *SubmitQueueConfig) {
	cmd.Flags().BoolVar(&config.Once, "once", false, "If true, run one loop and exit")
	cmd.Flags().StringSliceVar(&config.JenkinsJobs, "jenkins-jobs", []string{"kubernetes-e2e-gce", "kubernetes-e2e-gke-ci", "kubernetes-build", "kubernetes-e2e-gce-parallel", "kubernetes-e2e-gce-autoscaling", "kubernetes-e2e-gce-reboot", "kubernetes-e2e-gce-scalability"}, "Comma separated list of jobs in Jenkins to use for stability testing")
	cmd.Flags().StringVar(&config.JenkinsHost, "jenkins-host", "", "The URL for the jenkins job to watch")
	cmd.Flags().StringSliceVar(&config.RequiredStatusContexts, "required-contexts", []string{"cla/google", "Shippable", "continuous-integration/travis-ci/pr"}, "Comma separate list of status contexts required for a PR to be considered ok to merge")
	cmd.Flags().DurationVar(&config.PollPeriod, "poll-period", 30*time.Minute, "The period for running the submit-queue")
	cmd.Flags().StringVar(&config.Address, "address", ":8080", "The address to listen on for HTTP Status")
	cmd.Flags().StringVar(&config.DontRequireE2ELabel, "dont-require-e2e-label", "e2e-not-required", "If non-empty, a PR with this label will be merged automatically without looking at e2e results")
	cmd.Flags().StringVar(&config.E2EStatusContext, "e2e-status-context", "Jenkins GCE e2e", "The name of the github status context for the e2e PR Builder")
	cmd.Flags().StringVar(&config.WWWRoot, "www", "", "Path to static web files to serve from the webserver")
	cmd.PersistentFlags().AddGoFlagSet(goflag.CommandLine)
}

func (config *SubmitQueueConfig) validateLGTMAfterPush(pr *github_api.PullRequest, lastModifiedTime *time.Time) (bool, error) {
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

func (config *SubmitQueueConfig) handlePR(e2e *e2eTester, pr *github_api.PullRequest, issue *github_api.Issue) {
	userSet := config.userWhitelist

	if !github.HasLabel(issue.Labels, config.WhitelistOverride) && !userSet.Has(*pr.User.Login) {
		glog.V(4).Infof("Dropping %d since %s isn't in whitelist and %s isn't present", *pr.Number, *pr.User.Login, config.WhitelistOverride)
		if !github.HasLabel(issue.Labels, needsOKToMergeLabel) {
			config.AddLabels(*pr.Number, []string{needsOKToMergeLabel})
			body := "The author of this PR is not in the whitelist for merge, can one of the admins add the 'ok-to-merge' label?"
			config.WriteComment(*pr.Number, body)
		}
		return
	}

	// Tidy up the issue list.
	if github.HasLabel(issue.Labels, needsOKToMergeLabel) {
		config.RemoveLabel(*pr.Number, needsOKToMergeLabel)
	}

	lastModifiedTime, err := config.LastModifiedTime(*pr.Number)
	if err != nil {
		glog.Errorf("Failed to get last modified time, skipping PR: %d", *pr.Number)
		return
	}
	if ok, err := config.validateLGTMAfterPush(pr, lastModifiedTime); err != nil {
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
	contexts := config.RequiredStatusContexts
	if len(config.DontRequireE2ELabel) == 0 || !github.HasLabel(issue.Labels, config.DontRequireE2ELabel) {
		contexts = append(contexts, config.E2EStatusContext)
	}
	if ok := config.IsStatusSuccess(pr, contexts); !ok {
		glog.Errorf("PR# %d CI status is not success", *pr.Number)
		return
	}

	if err := e2e.runE2ETests(pr, issue); err != nil {
		glog.Errorf("Error running e2e test: %v", err)
		return
	}

	return
}

func (config *SubmitQueueConfig) doSubmitQueue() error {
	if len(config.JenkinsHost) == 0 {
		glog.Fatalf("--jenkins-host is required.")
	}

	e2e := &e2eTester{
		Config: config,
		state: &ExternalState{
			BuildStatus: map[string]string{},
		},
	}
	if len(config.Address) > 0 {
		if len(config.WWWRoot) > 0 {
			http.Handle("/", http.FileServer(http.Dir(config.WWWRoot)))
		}
		http.Handle("/api", e2e)
		go http.ListenAndServe(config.Address, nil)
	}
	for {
		glog.Infof("Beginning PR scan...")
		nextRunStartTime := time.Now().Add(config.PollPeriod)
		wl := config.RefreshWhitelist()
		e2e.locked(func() { e2e.state.Whitelist = wl.List() })
		err := config.ForEachPRDo([]string{"lgtm", "cla: yes"}, func(pr *github_api.PullRequest, issue *github_api.Issue) error {
			if pr == nil {
				return nil
			}
			config.handlePR(e2e, pr, issue)
			return nil
		})
		if err != nil {
			glog.Errorf("Error getting candidate PRs: %v", err)
		}
		config.ResetAPICount()
		if config.Once {
			break
		}
		if nextRunStartTime.After(time.Now()) {
			sleepDuration := nextRunStartTime.Sub(time.Now())
			glog.Infof("Sleeping for %v\n", sleepDuration)
			time.Sleep(sleepDuration)
		} else {
			glog.Infof("Not sleeping as we took more than %v to complete one loop\n", config.PollPeriod)
		}
	}
	return nil
}

func main() {
	config := &SubmitQueueConfig{}

	root := &cobra.Command{
		Use:   filepath.Base(os.Args[0]),
		Short: "A program to automatically merge PRs which meet certain criteria",
		RunE: func(_ *cobra.Command, _ []string) error {
			if err := config.PreExecute(); err != nil {
				return err
			}
			return config.doSubmitQueue()
		},
	}
	config.AddRootFlags(root)
	addSubmitFlags(root, config)

	addWhitelistCommand(root, config)

	if err := root.Execute(); err != nil {
		glog.Fatalf("%v\n", err)
	}
}
