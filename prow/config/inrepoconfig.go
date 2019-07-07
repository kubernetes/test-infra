// We need to put the inrepoconfig code into the config package to not cause an import cycle,
// because inrepoconfigs types import types from the config package and the config package needs
// the inrepoconfig types in `GetPresubmits`
package config

import (
	"k8s.io/test-infra/prow/git"
)

type InrepoconfigConfiguration struct {
	Enabled bool `json:"enabled"`
}

type Inrepoconfig struct {
	Presubmits []Presubmit `json:"presubmit,omitempty"`
}

func (c *ProwConfig) InRepoConfigConfiguration(org, repo string) InrepoconfigConfiguration {
	// Check on repo, org and global level
}

// This is for callers who want the Presubmits for all repos and don't know which
// repos exist. Even if inrepoconfig is enabled, this wont return presubmits
// in prow.yaml. This should only be used by components that do something with
// Presubmits but do not get triggered by PullRequests, e.G. Branchprotector and
// the config parsing
func (c *Config) GetStaticPresubmitsForAllRepos() map[string][]Presubmit {
	return c.presubmits
}

// This has to be used when not using GitHub, as the git.Client only supports Github.
// If your component uses Github, use Presubmits instead
func (c *Config) GetStaticPresubmits(identifier string) []Presumit {
	return c.presubmits[identifier]
}

// Used for all consumers of the Presubmits config that get triggered on pull requests.
// It can only be used with Github, as the *git.Client only works with GitHub
func (c *Config) Presubmits(gc *git.Client, org, repo, baseSHA string, headRefs []string) ([]Presubmit, error) {
	if !c.InRepoConfigConfiguration(org, repo).Enabled {
		return c.presubmits[org+"/"+repo], nil
	}

	inrepoconfig, err := c.GetInrepoconfig(gc, org, repo, baseRef, headRefs)
	if err != nil {
		return nil, err
	}

	return append(c.presubmits[org+"/"+repo], inrepoconfig.Presubmits...), nil
}

func (c *Config) getInrepoconfig(gc *git.Client, org, repo, baseRef string, headRefs []string) (*Inrepoconfig, error) {
	// * use git.Client to clone the repo and checkout baseRef
	// * if len(headRefs) > 0, merge them onto baseRef in order, using the
	//   mergeStrategy configured on tide
	// * check if prow.yaml exists, if not return
	// * read prow.yaml
	// * unmarshal prow.yaml
	// * default and validate presubmits in prow.yaml
}
