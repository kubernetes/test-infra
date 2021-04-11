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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
)

const (
	// PluginName is the name of the trigger plugin
	PluginName = "trigger"
)

// untrustedReason represents a combination (by ORing the appropriate consts) of reasons
// why a user is not trusted by TrustedUser. It is used to generate messaging for users.
type untrustedReason int

const (
	notMember untrustedReason = 1 << iota
	notCollaborator
	notSecondaryMember
)

// String constructs a string explaining the reason for a user's denial of trust
// from untrustedReason as described above.
func (u untrustedReason) String() string {
	var response string
	if u&notMember != 0 {
		response += "User is not a member of the org. "
	}
	if u&notCollaborator != 0 {
		response += "User is not a collaborator. "
	}
	if u&notSecondaryMember != 0 {
		response += "User is not a member of the trusted secondary org. "
	}
	response += "Satisfy at least one of these conditions to make the user trusted."
	return response
}

func init() {
	plugins.RegisterGenericCommentHandler(PluginName, handleGenericCommentEvent, helpProvider)
	plugins.RegisterPullRequestHandler(PluginName, handlePullRequest, helpProvider)
	plugins.RegisterPushEventHandler(PluginName, handlePush, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []config.OrgRepo) (*pluginhelp.PluginHelp, error) {
	configInfo := map[string]string{}
	for _, repo := range enabledRepos {
		trigger := config.TriggerFor(repo.Org, repo.Repo)
		org := repo.Org
		if trigger.TrustedOrg != "" {
			org = trigger.TrustedOrg
		}
		configInfo[repo.String()] = fmt.Sprintf("The trusted GitHub organization for this repository is %q.", org)
	}
	yamlSnippet, err := plugins.CommentMap.GenYaml(&plugins.Configuration{
		Triggers: []plugins.Trigger{
			{
				Repos: []string{
					"org/repo1",
					"org/repo2",
				},
				JoinOrgURL:     "https://github.com/kubernetes/community/blob/master/community-membership.md",
				OnlyOrgMembers: true,
				IgnoreOkToTest: true,
			},
		},
	})
	if err != nil {
		logrus.WithError(err).Warnf("cannot generate comments for %s plugin", PluginName)
	}
	pluginHelp := &pluginhelp.PluginHelp{
		Description: `The trigger plugin starts tests in reaction to commands and pull request events. It is responsible for ensuring that test jobs are only run on trusted PRs. A PR is considered trusted if the author is a member of the 'trusted organization' for the repository or if such a member has left an '/ok-to-test' command on the PR.
<br>Trigger starts jobs automatically when a new trusted PR is created or when an untrusted PR becomes trusted, but it can also be used to start jobs manually via the '/test' command.
<br>The '/retest' command can be used to rerun jobs that have reported failure.`,
		Config:  configInfo,
		Snippet: yamlSnippet,
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/ok-to-test",
		Description: "Marks a PR as 'trusted' and starts tests.",
		Featured:    false,
		WhoCanUse:   "Members of the trusted organization for the repo.",
		Examples:    []string{"/ok-to-test"},
	})
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/test [<job name>|all]",
		Description: "Manually starts a/all test job(s). Lists all possible job(s) when no jobs/an invalid job are specified.",
		Featured:    true,
		WhoCanUse:   "Anyone can trigger this command on a trusted PR.",
		Examples:    []string{"/test all", "/test pull-bazel-test"},
	})
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/retest",
		Description: "Rerun test jobs that have failed.",
		Featured:    true,
		WhoCanUse:   "Anyone can trigger this command on a trusted PR.",
		Examples:    []string{"/retest"},
	})
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/test ?",
		Description: "List available test job(s) for a trusted PR.",
		Featured:    true,
		WhoCanUse:   "Anyone can trigger this command on a trusted PR.",
		Examples:    []string{"/test ?"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	AddLabel(org, repo string, number int, label string) error
	BotUserChecker() (func(candidate string) bool, error)
	IsCollaborator(org, repo, user string) (bool, error)
	IsMember(org, user string) (bool, error)
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetRef(org, repo, ref string) (string, error)
	CreateComment(owner, repo string, number int, comment string) error
	ListIssueComments(owner, repo string, issue int) ([]github.IssueComment, error)
	CreateStatus(owner, repo, ref string, status github.Status) error
	GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error)
	GetPullRequestChanges(org, repo string, number int) ([]github.PullRequestChange, error)
	RemoveLabel(org, repo string, number int, label string) error
	DeleteStaleComments(org, repo string, number int, comments []github.IssueComment, isStale func(github.IssueComment) bool) error
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
}

type trustedPullRequestClient interface {
	GetIssueLabels(org, repo string, number int) ([]github.Label, error)
	IsMember(org, user string) (bool, error)
	IsCollaborator(org, repo, user string) (bool, error)
}

type prowJobClient interface {
	Create(context.Context, *prowapi.ProwJob, metav1.CreateOptions) (*prowapi.ProwJob, error)
	List(ctx context.Context, opts metav1.ListOptions) (*prowapi.ProwJobList, error)
	Update(context.Context, *prowapi.ProwJob, metav1.UpdateOptions) (*prowapi.ProwJob, error)
}

// Client holds the necessary structures to work with prow via logging, github, kubernetes and its configuration.
//
// TODO(fejta): consider exporting an interface rather than a struct
type Client struct {
	GitHubClient  githubClient
	ProwJobClient prowJobClient
	Config        *config.Config
	Logger        *logrus.Entry
	GitClient     git.ClientFactory
}

// trustedUserClient is used to check is user member and repo collaborator
type trustedUserClient interface {
	IsCollaborator(org, repo, user string) (bool, error)
	IsMember(org, user string) (bool, error)
}

func getClient(pc plugins.Agent) Client {
	return Client{
		GitHubClient:  pc.GitHubClient,
		Config:        pc.Config,
		ProwJobClient: pc.ProwJobClient,
		Logger:        pc.Logger,
		GitClient:     pc.GitClient,
	}
}

func handlePullRequest(pc plugins.Agent, pr github.PullRequestEvent) error {
	org, repo, _ := orgRepoAuthor(pr.PullRequest)
	return handlePR(getClient(pc), pc.PluginConfig.TriggerFor(org, repo), pr)
}

func handleGenericCommentEvent(pc plugins.Agent, gc github.GenericCommentEvent) error {
	return handleGenericComment(getClient(pc), pc.PluginConfig.TriggerFor(gc.Repo.Owner.Login, gc.Repo.Name), gc)
}

func handlePush(pc plugins.Agent, pe github.PushEvent) error {
	return handlePE(getClient(pc), pe)
}

// TrustedUserResponse is a response from TrustedUser. It contains the boolean response for trust as well
// a reason for denial if the user is not trusted.
type TrustedUserResponse struct {
	IsTrusted bool
	// Reason contains the reason that a user is not trusted if IsTrusted is false
	Reason string
}

// TrustedUser returns true if user is trusted in repo.
// Trusted users are either repo collaborators, org members or trusted org members.
func TrustedUser(ghc trustedUserClient, onlyOrgMembers bool, trustedOrg, user, org, repo string) (TrustedUserResponse, error) {
	errorResponse := TrustedUserResponse{IsTrusted: false}
	okResponse := TrustedUserResponse{IsTrusted: true}

	// First check if user is a collaborator, assuming this is allowed
	if !onlyOrgMembers {
		if ok, err := ghc.IsCollaborator(org, repo, user); err != nil {
			return errorResponse, fmt.Errorf("error in IsCollaborator: %v", err)
		} else if ok {
			return okResponse, nil
		}
	}

	// TODO(fejta): consider dropping support for org checks in the future.

	// Next see if the user is an org member
	if member, err := ghc.IsMember(org, user); err != nil {
		return errorResponse, fmt.Errorf("error in IsMember(%s): %v", org, err)
	} else if member {
		return okResponse, nil
	}

	// Determine if there is a second org to check. If there is no secondary org or they are the same, the result
	// is the same because the user already failed the check for the primary org.
	if trustedOrg == "" || trustedOrg == org {
		// the if/else is only to improve error messaging
		if onlyOrgMembers {
			return TrustedUserResponse{IsTrusted: false, Reason: notMember.String()}, nil // No trusted org and/or it is the same
		}
		return TrustedUserResponse{IsTrusted: false, Reason: (notMember | notCollaborator).String()}, nil // No trusted org and/or it is the same
	}

	// Check the second trusted org.
	member, err := ghc.IsMember(trustedOrg, user)
	if err != nil {
		return errorResponse, fmt.Errorf("error in IsMember(%s): %v", trustedOrg, err)
	} else if member {
		return okResponse, nil
	}

	// the if/else is only to improve error messaging
	if onlyOrgMembers {
		return TrustedUserResponse{IsTrusted: false, Reason: (notMember | notSecondaryMember).String()}, nil
	}
	return TrustedUserResponse{IsTrusted: false, Reason: (notMember | notSecondaryMember | notCollaborator).String()}, nil
}

// validateContextOverlap ensures that there will be no overlap in contexts between a set of jobs running and a set to skip
func validateContextOverlap(toRun, toSkip []config.Presubmit) error {
	requestedContexts := sets.NewString()
	for _, job := range toRun {
		requestedContexts.Insert(job.Context)
	}
	skippedContexts := sets.NewString()
	for _, job := range toSkip {
		skippedContexts.Insert(job.Context)
	}
	if overlap := requestedContexts.Intersection(skippedContexts).List(); len(overlap) > 0 {
		return fmt.Errorf("the following contexts are both triggered and skipped: %s", strings.Join(overlap, ", "))
	}

	return nil
}

// RunRequested executes the config.Presubmits that are requested
func RunRequested(c Client, pr *github.PullRequest, baseSHA string, requestedJobs []config.Presubmit, eventGUID string) error {
	return runRequested(c, pr, baseSHA, requestedJobs, eventGUID)
}

func runRequested(c Client, pr *github.PullRequest, baseSHA string, requestedJobs []config.Presubmit, eventGUID string, millisecondOverride ...time.Duration) error {
	var errors []error
	for _, job := range requestedJobs {
		c.Logger.Infof("Starting %s build.", job.Name)
		pj := pjutil.NewPresubmit(*pr, baseSHA, job, eventGUID)
		c.Logger.WithFields(pjutil.ProwJobFields(&pj)).Info("Creating a new prowjob.")
		if err := createWithRetry(context.TODO(), c.ProwJobClient, &pj, millisecondOverride...); err != nil {
			c.Logger.WithError(err).Error("Failed to create prowjob.")
			errors = append(errors, err)
		}
	}
	return utilerrors.NewAggregate(errors)
}

func getPresubmits(log *logrus.Entry, gc git.ClientFactory, cfg *config.Config, orgRepo string, baseSHAGetter, headSHAGetter config.RefGetter) []config.Presubmit {
	presubmits, err := cfg.GetPresubmits(gc, orgRepo, baseSHAGetter, headSHAGetter)
	if err != nil {
		// Fall back to static presubmits to avoid deadlocking when a presubmit is used to verify
		// inrepoconfig. Tide will still respect errors here and not merge.
		log.WithError(err).Debug("Failed to get presubmits")
		presubmits = cfg.PresubmitsStatic[orgRepo]
	}
	return presubmits
}

func getPostsubmits(log *logrus.Entry, gc git.ClientFactory, cfg *config.Config, orgRepo string, baseSHAGetter config.RefGetter) []config.Postsubmit {
	postsubmits, err := cfg.GetPostsubmits(gc, orgRepo, baseSHAGetter)
	if err != nil {
		// Fall back to static postsubmits, loading inrepoconfig returned an error.
		log.WithError(err).Error("Failed to get postsubmits")
		postsubmits = cfg.PostsubmitsStatic[orgRepo]
	}
	return postsubmits
}

// createWithRetry will retry the cration of a ProwJob. The Name must be set, otherwise we might end up creating it multiple times
// if one Create request errors but succeeds under the hood.
func createWithRetry(ctx context.Context, client prowJobClient, pj *prowapi.ProwJob, millisecondOverride ...time.Duration) error {
	millisecond := time.Millisecond
	if len(millisecondOverride) == 1 {
		millisecond = millisecondOverride[0]
	}

	var errs []error
	if err := wait.ExponentialBackoff(wait.Backoff{Duration: 250 * millisecond, Factor: 2.0, Jitter: 0.1, Steps: 8}, func() (bool, error) {
		if _, err := client.Create(ctx, pj, metav1.CreateOptions{}); err != nil {
			// Can happen if a previous request was successful but returned an error
			if apierrors.IsAlreadyExists(err) {
				return true, nil
			}
			// Store and swallow errors, if we end up timing out we will return all of them
			errs = append(errs, err)
			return false, nil
		}
		return true, nil
	}); err != nil {
		if err != wait.ErrWaitTimeout {
			return err
		}
		return utilerrors.NewAggregate(errs)
	}

	return nil
}
