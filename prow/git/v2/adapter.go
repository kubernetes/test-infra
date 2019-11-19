package git

import (
	"errors"
	"fmt"
	"strings"

	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
)

func OrgRepo(full string) (string, string, error) {
	if strings.Count(full, "/") != 1 {
		return "", "", fmt.Errorf("full repo name %s does not follow the org/repo format", full)
	}
	parts := strings.Split(full, "/")
	return parts[0], parts[1], nil
}

// ClientFactoryFrom adapts the v1 client to a v2 client
func ClientFactoryFrom(c *git.Client) ClientFactory {
	return &clientFactoryAdapter{Client: c}
}

type clientFactoryAdapter struct {
	*git.Client
}

// ClientFromDir creates a client that operates on a repo that has already
// been cloned to the given directory.
func (a *clientFactoryAdapter) ClientFromDir(org, repo, dir string) (RepoClient, error) {
	return nil, errors.New("no ClientFromDir implementation exists in the v1 git client")
}

// Repo creates a client that operates on a new clone of the repo.
func (a *clientFactoryAdapter) ClientFor(org, repo string) (RepoClient, error) {
	r, err := a.Client.Clone(org, repo)
	return &repoClientAdapter{Repo: r}, err
}

type repoClientAdapter struct {
	*git.Repo
}

func (a *repoClientAdapter) MergeAndCheckout(baseSHA string, headSHAs []string, mergeStrategy string) error {
	return a.Repo.MergeAndCheckout(baseSHA, headSHAs, github.PullRequestMergeType(mergeStrategy))
}

func (a *repoClientAdapter) MergeWithStrategy(commitlike, mergeStrategy string) (bool, error) {
	return a.Repo.MergeWithStrategy(commitlike, github.PullRequestMergeType(mergeStrategy))
}

func (a *repoClientAdapter) Clone(from string) error {
	return errors.New("no Clone implementation exists in the v1 repo client")
}

func (a *repoClientAdapter) Commit(title, body string) error {
	return errors.New("no Commit implementation exists in the v1 repo client")
}

func (a *repoClientAdapter) ForcePush(branch string) error {
	return a.Repo.Push(branch)
}

func (a *repoClientAdapter) MirrorClone() error {
	return errors.New("no MirrorClone implementation exists in the v1 repo client")
}

func (a *repoClientAdapter) Fetch() error {
	return errors.New("no Fetch implementation exists in the v1 repo client")
}

func (a *repoClientAdapter) RemoteUpdate() error {
	return errors.New("no RemoteUpdate implementation exists in the v1 repo client")
}

func (a *repoClientAdapter) FetchRef(refspec string) error {
	return errors.New("no FetchRef implementation exists in the v1 repo client")
}
