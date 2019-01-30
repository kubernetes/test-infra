package forcemerge

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"k8s.io/test-infra/prow/config"

	"github.com/ghodss/yaml"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/repoowners"

	okrov1beta2 "github.com/traiana/okro/okro/api/v1beta2"
)

const (
	pluginName         = "okro/force-merge"
	masterRef          = "heads/master"
	domainFile         = "domain.yaml"
	validateJobContext = "validate-domain"
)

var (
	mergeRe = regexp.MustCompile(`(?mi)^/force merge\s*$`)
)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericCommentEvent, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "By default, PRs to domain repos which break other domains are not allowed. " +
			"The force-merge plugin allows root approvers to merge such PRs.",
	}
	pluginHelp.AddCommand(pluginhelp.Command{
		Usage:       "/force merge",
		Description: "Confirms and merges the PR.",
		Featured:    true,
		WhoCanUse:   "Root approvers.",
		Examples:    []string{"/force merge"},
	})
	return pluginHelp, nil
}

type githubClient interface {
	CreateComment(owner, repo string, number int, comment string) error
	GetPullRequest(org, repo string, number int) (*github.PullRequest, error)
	GetFile(org, repo, filepath, commit string) ([]byte, error)
	Merge(org, repo string, pr int, details github.MergeDetails) error
	GetCombinedStatus(org, repo, ref string) (*github.CombinedStatus, error)
	GetRef(org, repo, ref string) (string, error)
	CreateStatus(org, repo, SHA string, s github.Status) error
}

type ownersClient interface {
	LoadRepoOwners(org, repo, base string) (repoowners.RepoOwner, error)
}

type okroClient interface {
	GetTenant(tenant string) (*okrov1beta2.Tenant, error)
	ValidateDomain(tenant string, domain *okrov1beta2.Domain, commit string) error
}

func handleGenericCommentEvent(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handleGenericComment(pc.Logger, pc.OkroConfig, pc.GitHubClient, pc.OwnersClient, pc.OkroClient, &e)
}

func handleGenericComment(log *logrus.Entry, okroConfig *config.OkroConfig, ghc githubClient, oc ownersClient, okroClient okroClient, ce *github.GenericCommentEvent) error {
	// Only consider open PRs and new comments.
	if !ce.IsPR || ce.IssueState != "open" || ce.Action != github.GenericCommentActionCreated || !mergeRe.MatchString(ce.Body) {
		return nil
	}

	org := ce.Repo.Owner.Login
	repo := ce.Repo.Name
	pr, err := ghc.GetPullRequest(org, repo, ce.Number)
	if err != nil {
		return fmt.Errorf("failed to get PR %s/%s#%d: %s", org, repo, ce.Number, err)
	}

	// Check user is root approver
	owners, err := oc.LoadRepoOwners(org, repo, pr.Base.Ref)
	if err != nil {
		return fmt.Errorf("failed to load repo owners for PR %s/%s#%d: %s", org, repo, ce.Number, err)
	}
	approvers := owners.Approvers("OWNERS")
	if !approvers.Has(ce.User.Login) {
		msg := fmt.Sprintf("Only root approvers are allowed to force-merge this PR.\nRoot approvers are: %s.",
			strings.Join(approvers.List(), ", "))
		return createResponseComment(ghc, ce, msg)
	}

	// Check PR is synced with master. This is important because unlike in presubmit jobs, we validate the PR
	// without merging it to master first, which means the PR may be valid but will break master once it's merged.
	masterSHA, err := ghc.GetRef(org, repo, masterRef)
	if err != nil {
		return fmt.Errorf("failed to get %s ref for repo %s/%s: %s", masterRef, org, repo, err)
	}
	if masterSHA != pr.Base.SHA {
		msg := "This branch is out-of-date with the base branch.\n" +
			"Merge the latest changes from `master` into this branch and run `/force merge` again."
		return createResponseComment(ghc, ce, msg)
	}

	// Do validation
	b, err := ghc.GetFile(org, repo, domainFile, pr.Head.SHA)
	var invalidMessage string
	var warningsDesc string
	if err != nil {
		if _, fileNotFound := err.(*github.FileNotFound); !fileNotFound {
			return fmt.Errorf("failed to get domain file in %s/%s#%d", org, repo, ce.Number)
		}
		invalidMessage = fmt.Sprintf("%s not found.", domainFile)
	}

	if invalidMessage == "" {
		var domain *okrov1beta2.Domain
		if err := yaml.Unmarshal(b, &domain); err != nil {
			invalidMessage = fmt.Sprintf("invalid %s format.", domainFile)
		} else {
			tenant, err := okroClient.GetTenant(okroConfig.Tenant)
			if err != nil {
				return fmt.Errorf("failed to get tenant %s", okroConfig.Tenant)
			}
			var env string
			repoURLSuffix := fmt.Sprintf("%s/%s.git", org, repo)
			for _, domainURL := range tenant.DomainURLs {
				if strings.HasSuffix(domainURL.URL, repoURLSuffix) {
					env = domainURL.Env
					break
				}
			}
			if env == "" {
				return fmt.Errorf("failed to get env for repo %s/%s", org, repo)
			}
			domain.Name = env
			err = okroClient.ValidateDomain(tenant.Name, domain, pr.Base.SHA)
			if err == nil {
				invalidMessage = "repo is in a valid state.\nIf the validation job has failed, run `/retest`. " +
					"Otherwise, wait for the PR to be merged automatically."
			} else {
				okroErr, ok := err.(okrov1beta2.Error)
				if !ok {
					return fmt.Errorf("failed to validate domain %s/%s: %v", tenant, domain, err)
				}
				// TODO: use AppCode
				if okroErr.Message == "succeeded with warnings" {
					warningsDesc, err = asCodeSection(okroErr.Details)
					if err != nil {
						return fmt.Errorf("failed to marshal warnings to json")
					}
				} else {
					invalidMessage = okroErr.Message
					if okroErr.Details != nil {
						if code, err := asCodeSection(okroErr.Details); err != nil {
							invalidMessage += fmt.Sprintf("\n\n%s", code)
						}
					}
				}
			}
		}
	}

	if invalidMessage != "" {
		msg := fmt.Sprintf("Merge is not allowed: %s", invalidMessage)
		return createResponseComment(ghc, ce, msg)
	}
	msg := fmt.Sprintf("Forcing merge ignoring the following warnings:\n\n%s", warningsDesc)
	if err := createResponseComment(ghc, ce, msg); err != nil {
		return err
	}
	if err := ghc.CreateStatus(org, repo, pr.Head.SHA, github.Status{State: "success", Context: validateJobContext}); err != nil {
		return fmt.Errorf("failed to create status in %s/%s#%d", org, repo, ce.Number)
	}
	if err := ghc.Merge(org, repo, pr.Number, github.MergeDetails{SHA: pr.Head.SHA}); err != nil {
		return createResponseComment(ghc, ce, fmt.Sprintf("Merge failed: %v", err))
	}
	return nil
}

func createResponseComment(ghc githubClient, ce *github.GenericCommentEvent, msg string) error {
	comment := plugins.FormatResponseRaw(ce.Body, ce.HTMLURL, ce.User.Login, msg)
	if err := ghc.CreateComment(ce.Repo.Owner.Login, ce.Repo.Name, ce.Number, comment); err != nil {
		return fmt.Errorf("failed to create comment in %s/%s#%d", ce.Repo.Owner.Login, ce.Repo.Name, ce.Number)
	}
	return nil
}

func asCodeSection(data interface{}) (string, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("```\n%s\n```", string(b)), nil
}
