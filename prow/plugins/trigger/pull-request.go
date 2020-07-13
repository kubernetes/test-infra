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
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	klabels "k8s.io/apimachinery/pkg/labels"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/plugins"
)

func handlePR(c Client, trigger plugins.Trigger, pr github.PullRequestEvent) error {
	org, repo, a := orgRepoAuthor(pr.PullRequest)
	author := string(a)
	num := pr.PullRequest.Number

	baseSHA := ""
	baseSHAGetter := func() (string, error) {
		var err error
		baseSHA, err = c.GitHubClient.GetRef(org, repo, "heads/"+pr.PullRequest.Base.Ref)
		if err != nil {
			return "", fmt.Errorf("failed to get baseSHA: %v", err)
		}
		return baseSHA, nil
	}
	headSHAGetter := func() (string, error) {
		return pr.PullRequest.Head.SHA, nil
	}

	presubmits := getPresubmits(c.Logger, c.GitClient, c.Config, org+"/"+repo, baseSHAGetter, headSHAGetter)
	if len(presubmits) == 0 {
		return nil
	}

	if baseSHA == "" {
		if _, err := baseSHAGetter(); err != nil {
			return err
		}
	}

	switch pr.Action {
	case github.PullRequestActionOpened:
		// When a PR is opened, if the author is in the org then build it.
		// Otherwise, ask for "/ok-to-test". There's no need to look for previous
		// "/ok-to-test" comments since the PR was just opened!
		trustedResponse, err := TrustedUser(c.GitHubClient, trigger.OnlyOrgMembers, trigger.TrustedOrg, author, org, repo)
		member := trustedResponse.IsTrusted
		if err != nil {
			return fmt.Errorf("could not check membership: %s", err)
		}
		if member {
			// dedicated draft check for create to comment on the PR
			if pr.PullRequest.Draft {
				c.Logger.Info("Skipping all jobs for draft PR.")
				return draftMsg(c.GitHubClient, pr.PullRequest)
			}
			c.Logger.Info("Starting all jobs for new PR.")
			return buildAllButDrafts(c, &pr.PullRequest, pr.GUID, baseSHA, presubmits)
		}
		c.Logger.Infof("Welcome message to PR author %q.", author)
		if err := welcomeMsg(c.GitHubClient, trigger, pr.PullRequest); err != nil {
			return fmt.Errorf("could not welcome non-org member %q: %v", author, err)
		}
	case github.PullRequestActionReopened:
		return buildAllIfTrusted(c, trigger, pr, baseSHA, presubmits)
	case github.PullRequestActionEdited:
		// if someone changes the base of their PR, we will get this
		// event and the changes field will list that the base SHA and
		// ref changes so we can detect such a case and retrigger tests
		var changes struct {
			Base struct {
				Ref struct {
					From string `json:"from"`
				} `json:"ref"`
				Sha struct {
					From string `json:"from"`
				} `json:"sha"`
			} `json:"base"`
		}
		if err := json.Unmarshal(pr.Changes, &changes); err != nil {
			// we're detecting this best-effort so we can forget about
			// the event
			return nil
		} else if changes.Base.Ref.From != "" || changes.Base.Sha.From != "" {
			// the base of the PR changed and we need to re-test it
			return buildAllIfTrusted(c, trigger, pr, baseSHA, presubmits)
		}
	case github.PullRequestActionSynchronize:
		return buildAllIfTrusted(c, trigger, pr, baseSHA, presubmits)
	case github.PullRequestActionLabeled:
		// When a PR is LGTMd, if it is untrusted then build it once.
		if pr.Label.Name == labels.LGTM {
			_, trusted, err := TrustedPullRequest(c.GitHubClient, trigger, author, org, repo, num, nil)
			if err != nil {
				return fmt.Errorf("could not validate PR: %s", err)
			} else if !trusted {
				c.Logger.Info("Starting all jobs for untrusted PR with LGTM.")
				return buildAllButDrafts(c, &pr.PullRequest, pr.GUID, baseSHA, presubmits)
			}
		}
		if pr.Label.Name == labels.OkToTest {
			// When the bot adds the label from an /ok-to-test command,
			// we will trigger tests based on the comment event and do not
			// need to trigger them here from the label, as well
			botName, err := c.GitHubClient.BotName()
			if err != nil {
				return err
			}
			if author == botName {
				c.Logger.Debug("Label added by the bot, skipping.")
				return nil
			}
			return buildAllButDrafts(c, &pr.PullRequest, pr.GUID, baseSHA, presubmits)
		}
	case github.PullRequestActionClosed:
		if err := abortAllJobs(c, &pr.PullRequest); err != nil {
			c.Logger.WithError(err).Error("Failed to abort jobs for closed pull request")
			return err
		}
	case github.PullRequestActionReadyForReview:
		return buildAllIfTrusted(c, trigger, pr, baseSHA, presubmits)
	case github.PullRequestConvertedToDraft:
		if err := abortAllJobs(c, &pr.PullRequest); err != nil {
			c.Logger.WithError(err).Error("Failed to abort jobs for pull request converted to draft")
			return err
		}
	}

	return nil
}

func abortAllJobs(c Client, pr *github.PullRequest) error {
	selector, err := labelSelectorForPR(pr)
	if err != nil {
		return fmt.Errorf("failed to construct label selector: %w", err)
	}

	jobs, err := c.ProwJobClient.List(metav1.ListOptions{LabelSelector: selector.String()})
	if err != nil {
		return fmt.Errorf("failed to list prowjobs for pr: %w", err)
	}

	var errs []error
	for _, job := range jobs.Items {
		// Do not abort jobs that already completed
		if job.Complete() {
			continue
		}
		job.Status.State = prowapi.AbortedState
		// We use Update and not Patch here, because we are not the authority of the .Status.State field
		// and must not overwrite changes made to it in the interim by the responsible agent.
		// The accepted trade-off for now is that this leads to failure if unrelated fields where changed
		// by another different actor.
		if _, err := c.ProwJobClient.Update(&job); err != nil && !apierrors.IsConflict(err) {
			errs = append(errs, fmt.Errorf("failed to abort job %s: %w", job.Name, err))
		}
	}

	return utilerrors.NewAggregate(errs)
}

func labelSelectorForPR(pr *github.PullRequest) (klabels.Selector, error) {
	set := klabels.Set{
		kube.OrgLabel:         pr.Base.Repo.Owner.Login,
		kube.RepoLabel:        pr.Base.Repo.Name,
		kube.PullLabel:        strconv.Itoa(pr.Number),
		kube.ProwJobTypeLabel: string(prowapi.PresubmitJob),
	}
	selector := klabels.SelectorFromSet(set)
	// Needed because of this gem:
	// https://github.com/kubernetes/apimachinery/blob/f8e71527369e696bf041722b248ffcb32bae9edf/pkg/labels/selector.go#L883
	if selector.Empty() {
		return nil, errors.New("got back empty selector")
	}

	return selector, nil
}

type login string

func orgRepoAuthor(pr github.PullRequest) (string, string, login) {
	org := pr.Base.Repo.Owner.Login
	repo := pr.Base.Repo.Name
	author := pr.User.Login
	return org, repo, login(author)
}

func buildAllIfTrusted(c Client, trigger plugins.Trigger, pr github.PullRequestEvent, baseSHA string, presubmits []config.Presubmit) error {
	// When a PR is updated, check that the user is in the org or that an org
	// member has said "/ok-to-test" before building. There's no need to ask
	// for "/ok-to-test" because we do that once when the PR is created.
	org, repo, a := orgRepoAuthor(pr.PullRequest)
	author := string(a)
	num := pr.PullRequest.Number
	l, trusted, err := TrustedPullRequest(c.GitHubClient, trigger, author, org, repo, num, nil)
	if err != nil {
		return fmt.Errorf("could not validate PR: %s", err)
	} else if trusted {
		// Eventually remove needs-ok-to-test
		// Will not work for org members since labels are not fetched in this case
		if github.HasLabel(labels.NeedsOkToTest, l) {
			if err := c.GitHubClient.RemoveLabel(org, repo, num, labels.NeedsOkToTest); err != nil {
				return err
			}
		}
		c.Logger.Info("Starting all jobs for updated PR.")
		return buildAllButDrafts(c, &pr.PullRequest, pr.GUID, baseSHA, presubmits)
	}
	return nil
}

func welcomeMsg(ghc githubClient, trigger plugins.Trigger, pr github.PullRequest) error {
	var errors []error
	org, repo, a := orgRepoAuthor(pr)
	author := string(a)
	encodedRepoFullName := url.QueryEscape(pr.Base.Repo.FullName)
	var more string
	if trigger.TrustedOrg != "" && trigger.TrustedOrg != org {
		more = fmt.Sprintf("or [%s](https://github.com/orgs/%s/people) ", trigger.TrustedOrg, trigger.TrustedOrg)
	}

	var joinOrgURL string
	if trigger.JoinOrgURL != "" {
		joinOrgURL = trigger.JoinOrgURL
	} else {
		joinOrgURL = fmt.Sprintf("https://github.com/orgs/%s/people", org)
	}

	var comment string
	if trigger.IgnoreOkToTest {
		comment = fmt.Sprintf(`Hi @%s. Thanks for your PR.

PRs from untrusted users cannot be marked as trusted with `+"`/ok-to-test`"+` in this repo meaning untrusted PR authors can never trigger tests themselves. Collaborators can still trigger tests on the PR using `+"`/test all`"+`.

I understand the commands that are listed [here](https://go.k8s.io/bot-commands?repo=%s).

<details>

%s
</details>
`, author, encodedRepoFullName, plugins.AboutThisBotWithoutCommands)
	} else {
		comment = fmt.Sprintf(`Hi @%s. Thanks for your PR.

I'm waiting for a [%s](https://github.com/orgs/%s/people) %smember to verify that this patch is reasonable to test. If it is, they should reply with `+"`/ok-to-test`"+` on its own line. Until that is done, I will not automatically test new commits in this PR, but the usual testing commands by org members will still work. Regular contributors should [join the org](%s) to skip this step.

Once the patch is verified, the new status will be reflected by the `+"`%s`"+` label.

I understand the commands that are listed [here](https://go.k8s.io/bot-commands?repo=%s).

<details>

%s
</details>
`, author, org, org, more, joinOrgURL, labels.OkToTest, encodedRepoFullName, plugins.AboutThisBotWithoutCommands)
		if err := ghc.AddLabel(org, repo, pr.Number, labels.NeedsOkToTest); err != nil {
			errors = append(errors, err)
		}
	}

	if err := ghc.CreateComment(org, repo, pr.Number, comment); err != nil {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return utilerrors.NewAggregate(errors)
	}
	return nil
}

func draftMsg(ghc githubClient, pr github.PullRequest) error {
	org, repo, _ := orgRepoAuthor(pr)

	comment := "Skipping CI for Draft Pull Request.\nIf you want CI signal for your change, please convert it to an actual PR.\nYou can still manually trigger a test run with `/test all`"
	return ghc.CreateComment(org, repo, pr.Number, comment)
}

// TrustedPullRequest returns whether or not the given PR should be tested.
// It first checks if the author is in the org, then looks for "ok-to-test" label.
// If already known, GitHub labels should be provided to save tokens. Otherwise, it fetches them.
func TrustedPullRequest(tprc trustedPullRequestClient, trigger plugins.Trigger, author, org, repo string, num int, l []github.Label) ([]github.Label, bool, error) {
	// First check if the author is a member of the org.
	if trustedResponse, err := TrustedUser(tprc, trigger.OnlyOrgMembers, trigger.TrustedOrg, author, org, repo); err != nil {
		return l, false, fmt.Errorf("error checking %s for trust: %v", author, err)
	} else if trustedResponse.IsTrusted {
		return l, true, nil
	}
	// Then check if PR has ok-to-test label
	if l == nil {
		var err error
		l, err = tprc.GetIssueLabels(org, repo, num)
		if err != nil {
			return l, false, err
		}
	}
	return l, github.HasLabel(labels.OkToTest, l), nil
}

// buildAllButDrafts ensures that all builds that should run and will be required are built, but skips draft PRs
func buildAllButDrafts(c Client, pr *github.PullRequest, eventGUID string, baseSHA string, presubmits []config.Presubmit) error {
	if pr.Draft {
		c.Logger.Info("Skipping all jobs for draft PR.")
		return nil
	}
	return buildAll(c, pr, eventGUID, baseSHA, presubmits)
}

// buildAll ensures that all builds that should run and will be required are built
func buildAll(c Client, pr *github.PullRequest, eventGUID string, baseSHA string, presubmits []config.Presubmit) error {
	org, repo, number, branch := pr.Base.Repo.Owner.Login, pr.Base.Repo.Name, pr.Number, pr.Base.Ref
	changes := config.NewGitHubDeferredChangedFilesProvider(c.GitHubClient, org, repo, number)
	toTest, err := pjutil.FilterPresubmits(pjutil.TestAllFilter(), changes, branch, presubmits, c.Logger)
	if err != nil {
		return err
	}
	return RunRequested(c, pr, baseSHA, toTest, eventGUID)
}
