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

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/kube"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/plugins"
)

func handleGenericComment(c Client, trigger plugins.Trigger, gc github.GenericCommentEvent) error {
	org := gc.Repo.Owner.Login
	repo := gc.Repo.Name
	number := gc.Number
	commentAuthor := gc.User.Login
	// Only take action when a comment is first created,
	// when it belongs to a PR,
	// and the PR is open.
	if gc.Action != github.GenericCommentActionCreated || !gc.IsPR || gc.IssueState != "open" {
		return nil
	}

	// Skip bot comments.
	botUserChecker, err := c.GitHubClient.BotUserChecker()
	if err != nil {
		return err
	}

	if botUserChecker(commentAuthor) {
		c.Logger.Debug("Comment is made by the bot, skipping.")
		return nil
	}

	refGetter := config.NewRefGetterForGitHubPullRequest(c.GitHubClient, org, repo, number)
	presubmits := getPresubmits(c.Logger, c.GitClient, c.Config, org+"/"+repo, refGetter.BaseSHA, refGetter.HeadSHA)

	// Skip comments not germane to this plugin
	if !pjutil.RetestRe.MatchString(gc.Body) &&
		!pjutil.RetestRequiredRe.MatchString(gc.Body) &&
		!pjutil.OkToTestRe.MatchString(gc.Body) &&
		!pjutil.TestAllRe.MatchString(gc.Body) &&
		!pjutil.MayNeedHelpComment(gc.Body) {
		matched := false
		for _, presubmit := range presubmits {
			matched = matched || presubmit.TriggerMatches(gc.Body)
			if matched {
				break
			}
		}
		if !matched {
			c.Logger.Debug("Comment doesn't match any triggering regex, skipping.")
			return nil
		}
	}

	// Skip untrusted users comments.
	trustedResponse, err := TrustedUser(c.GitHubClient, trigger.OnlyOrgMembers, trigger.TrustedApps, trigger.TrustedOrg, commentAuthor, org, repo)
	if err != nil {
		return fmt.Errorf("error checking trust of %s: %w", commentAuthor, err)
	}

	trusted := trustedResponse.IsTrusted
	var l []github.Label
	if !trusted {
		// Skip untrusted PRs.
		l, trusted, err = TrustedPullRequest(c.GitHubClient, trigger, gc.IssueAuthor.Login, org, repo, number, nil)
		if err != nil {
			return err
		}
		if !trusted {
			resp := "Cannot trigger testing until a trusted user reviews the PR and leaves an `/ok-to-test` message."
			c.Logger.Infof("Commenting \"%s\".", resp)
			return c.GitHubClient.CreateComment(org, repo, number, plugins.FormatResponseRaw(gc.Body, gc.HTMLURL, gc.User.Login, resp))
		}
	}

	// At this point we can trust the PR, so we eventually update labels.
	// Ensure we have labels before test, because TrustedPullRequest() won't be called
	// when commentAuthor is trusted.
	if l == nil {
		l, err = c.GitHubClient.GetIssueLabels(org, repo, number)
		if err != nil {
			return err
		}
	}
	isOkToTest := HonorOkToTest(trigger) && pjutil.OkToTestRe.MatchString(gc.Body)
	if isOkToTest && !github.HasLabel(labels.OkToTest, l) {
		if err := c.GitHubClient.AddLabel(org, repo, number, labels.OkToTest); err != nil {
			return err
		}
	}
	if (isOkToTest || github.HasLabel(labels.OkToTest, l)) && github.HasLabel(labels.NeedsOkToTest, l) {
		if err := c.GitHubClient.RemoveLabel(org, repo, number, labels.NeedsOkToTest); err != nil {
			return err
		}
	}

	pr, err := refGetter.PullRequest()
	if err != nil {
		return err
	}
	baseSHA, err := refGetter.BaseSHA()
	if err != nil {
		return err
	}

	toTest, err := FilterPresubmits(HonorOkToTest(trigger), c.GitHubClient, gc.Body, pr, presubmits, c.Logger)
	if err != nil {
		return err
	}
	if needsHelp, note := pjutil.ShouldRespondWithHelp(gc.Body, len(toTest)); needsHelp {
		return addHelpComment(c.GitHubClient, gc.Body, org, repo, pr.Base.Ref, pr.Number, presubmits, gc.HTMLURL, commentAuthor, note, c.Logger)
	}
	// we want to be able to track re-tests separately from the general body of tests
	additionalLabels := map[string]string{}
	if pjutil.RetestRe.MatchString(gc.Body) || pjutil.RetestRequiredRe.MatchString(gc.Body) {
		additionalLabels[kube.RetestLabel] = "true"
	}
	// run failed github actions
	if trigger.TriggerGitHubWorkflows && (pjutil.RetestRe.MatchString(gc.Body) || pjutil.TestAllRe.MatchString(gc.Body)) {
		headSHA, err := refGetter.HeadSHA()
		if err != nil {
			c.Logger.Warnf("headSHA unvailable, failed github actions for pr will not be triggered: %v", pr)
		} else {
			failedRuns, err := c.GitHubClient.GetFailedActionRunsByHeadBranch(org, repo, pr.Head.Ref, headSHA)
			if err != nil {
				c.Logger.Errorf("%v: unable to get failed github action runs for branch %v", err, pr.Head.Ref)
			} else {
				for _, run := range failedRuns {
					log := c.Logger.WithFields(logrus.Fields{
						"runID":   run.ID,
						"runName": run.Name,
						"org":     org,
						"repo":    repo,
					})
					runID := run.ID
					go func() {
						if err := c.GitHubClient.TriggerFailedGitHubWorkflow(org, repo, runID); err != nil {
							log.Errorf("attempt to trigger github run failed: %v", err)
						} else {
							log.Infof("successfully triggered action run")
						}
					}()
				}
			}
		}
	}
	return RunRequestedWithLabels(c, pr, baseSHA, toTest, gc.GUID, additionalLabels)
}

func HonorOkToTest(trigger plugins.Trigger) bool {
	return !trigger.IgnoreOkToTest
}

type GitHubClient interface {
	GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
}

// FilterPresubmits determines which presubmits should run. We only want to
// trigger jobs that should run, but the pool of jobs we filter to those that
// should run depends on the type of trigger we just got:
//   - if we get a /test foo, we only want to consider those jobs that match;
//     jobs will default to run unless we can determine they shouldn't
//   - if we got a /retest, we only want to consider those jobs that have
//     already run and posted failing contexts to the PR or those jobs that
//     have not yet run but would otherwise match /test all; jobs will default
//     to run unless we can determine they shouldn't
//   - if we got a /test all or an /ok-to-test, we want to consider any job
//     that doesn't explicitly require a human trigger comment; jobs will
//     default to not run unless we can determine that they should
//
// If a comment that we get matches more than one of the above patterns, we
// consider the set of matching presubmits the union of the results from the
// matching cases.
func FilterPresubmits(honorOkToTest bool, gitHubClient GitHubClient, body string, pr *github.PullRequest, presubmits []config.Presubmit, logger *logrus.Entry) ([]config.Presubmit, error) {
	org, repo, sha := pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pr.Head.SHA

	contextGetter := func() (sets.Set[string], sets.Set[string], error) {
		combinedStatus, err := gitHubClient.GetCombinedStatus(org, repo, sha)
		if err != nil {
			return nil, nil, err
		}
		failedContexts, allContexts := getContexts(combinedStatus)
		return failedContexts, allContexts, nil
	}

	filter, err := pjutil.PresubmitFilter(honorOkToTest, contextGetter, body, logger)
	if err != nil {
		return nil, err
	}

	number, branch := pr.Number, pr.Base.Ref
	changes := config.NewGitHubDeferredChangedFilesProvider(gitHubClient, org, repo, number)
	return pjutil.FilterPresubmits(filter, changes, branch, presubmits, logger)
}

func getContexts(combinedStatus *github.CombinedStatus) (sets.Set[string], sets.Set[string]) {
	allContexts := sets.Set[string]{}
	failedContexts := sets.Set[string]{}
	if combinedStatus != nil {
		for _, status := range combinedStatus.Statuses {
			allContexts.Insert(status.Context)
			if status.State == github.StatusError || status.State == github.StatusFailure {
				failedContexts.Insert(status.Context)
			}
		}
	}
	return failedContexts, allContexts
}

func addHelpComment(githubClient githubClient, body, org, repo, branch string, number int, presubmits []config.Presubmit, HTMLURL, user, note string, logger *logrus.Entry) error {
	changes := config.NewGitHubDeferredChangedFilesProvider(githubClient, org, repo, number)
	testAllNames, optionalJobsCommands, requiredJobsCommands, err := pjutil.AvailablePresubmits(changes, branch, presubmits, logger)
	if err != nil {
		return err
	}

	resp := pjutil.HelpMessage(org, repo, branch, note, testAllNames, optionalJobsCommands, requiredJobsCommands)
	return githubClient.CreateComment(org, repo, number, plugins.FormatResponseRaw(body, HTMLURL, user, resp))
}
