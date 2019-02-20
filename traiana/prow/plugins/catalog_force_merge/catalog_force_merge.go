package catalogforcemerge

import (
	"fmt"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/repoowners"
	"k8s.io/test-infra/traiana/prow/force_merge"

	okrov1beta2 "github.com/traiana/okro/okro/api/v1beta2"
	"github.com/traiana/prow-jobs/pkg/construct"
	gitutils "github.com/traiana/prow-jobs/pkg/utils/git"
)

const (
	pluginName         = "okro/catalog-force-merge"
	validateJobContext = "catalog-validator"
)

func init() {
	plugins.RegisterGenericCommentHandler(pluginName, handleGenericCommentEvent, helpProvider)
}

func helpProvider(config *plugins.Configuration, enabledRepos []string) (*pluginhelp.PluginHelp, error) {
	// The Config field is omitted because this plugin is not configurable.
	pluginHelp := &pluginhelp.PluginHelp{
		Description: "By default, PRs to catalog repos which change dynamic selections in sensitive environments are " +
			"not allowed. The force-merge plugin allows root approvers to merge such PRs.",
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

type gitClient interface {
	Clone(repo string) (*git.Repo, error)
}

type ownersClient interface {
	LoadRepoOwners(org, repo, base string) (repoowners.RepoOwner, error)
}

type okroClient interface {
	ValidateCatalog(tenant string, catalog *okrov1beta2.Catalog, commit string) error
}

func handleGenericCommentEvent(pc plugins.Agent, e github.GenericCommentEvent) error {
	return handleGenericComment(pc.OkroConfig, pc.GitHubClient, pc.GitClient, pc.OwnersClient, pc.OkroClient, &e)
}

func handleGenericComment(okroConfig *config.OkroConfig, ghc githubClient, gc gitClient,
	oc ownersClient, okroClient okroClient, ce *github.GenericCommentEvent) error {
	return forcemerge.HandleGenericComment(ghc, gc, oc, ce, validateJobContext, func(clonedDir string, baseSHA string) error {
		catalog, err := construct.Catalog(clonedDir)
		if err != nil {
			return forcemerge.NewConstructError(fmt.Sprintf("failed to construct catalog: %v", err))
		}

		if err = okroClient.ValidateCatalog(okroConfig.Tenant, catalog, gitutils.Shorten(baseSHA)); err != nil {
			okroErr, ok := err.(okrov1beta2.Error)
			if !ok || !okroErr.IsClientFacing() {
				return fmt.Errorf("failed to validate catalog for tenant %s: %v", okroConfig.Tenant, err)
			}
			return okroErr
		}
		return nil
	})
}
