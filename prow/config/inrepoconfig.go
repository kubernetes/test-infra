/*
Copyright 2019 The Kubernetes Authors.

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

package config

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sync"

	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	"k8s.io/test-infra/prow/git/v2"
	"sigs.k8s.io/yaml"
)

const (
	inRepoConfigFileName = ".prow.yaml"
	inRepoConfigDirName  = ".prow"
)

// ProwYAML represents the content of a .prow.yaml file
// used to version Presubmits and Postsubmits inside the tested repo.
type ProwYAML struct {
	Presets     []Preset     `json:"presets"`
	Presubmits  []Presubmit  `json:"presubmits"`
	Postsubmits []Postsubmit `json:"postsubmits"`
}

// ProwYAMLGetter is used to retrieve a ProwYAML. Tests should provide
// their own implementation and set that on the Config.
type ProwYAMLGetter func(c *Config, gc git.ClientFactory, identifier, baseSHA string, headSHAs ...string) (*ProwYAML, error)

// Verify defaultProwYAMLGetter is a ProwYAMLGetter
var _ ProwYAMLGetter = defaultProwYAMLGetter

func defaultProwYAMLGetter(
	c *Config,
	gc git.ClientFactory,
	identifier string,
	baseSHA string,
	headSHAs ...string) (*ProwYAML, error) {

	log := logrus.WithField("repo", identifier)

	if gc == nil {
		log.Error("defaultProwYAMLGetter was called with a nil git client")
		return nil, errors.New("gitClient is nil")
	}

	orgRepo := *NewOrgRepo(identifier)
	if orgRepo.Repo == "" {
		return nil, fmt.Errorf("didn't get two results when splitting repo identifier %q", identifier)
	}
	repo, err := gc.ClientFor(orgRepo.Org, orgRepo.Repo)
	if err != nil {
		return nil, fmt.Errorf("failed to clone repo for %q: %v", identifier, err)
	}
	defer func() {
		if err := repo.Clean(); err != nil {
			log.WithError(err).Error("Failed to clean up repo.")
		}
	}()

	if err := repo.Config("user.name", "prow"); err != nil {
		return nil, err
	}
	if err := repo.Config("user.email", "prow@localhost"); err != nil {
		return nil, err
	}
	if err := repo.Config("commit.gpgsign", "false"); err != nil {
		return nil, err
	}

	mergeMethod := c.Tide.MergeMethod(orgRepo)
	log.Debugf("Using merge strategy %q.", mergeMethod)
	if err := repo.MergeAndCheckout(baseSHA, string(mergeMethod), headSHAs...); err != nil {
		return nil, fmt.Errorf("failed to merge: %v", err)
	}

	prowYAML := &ProwYAML{}

	prowYAMLDirPath := path.Join(repo.Directory(), inRepoConfigDirName)
	log.Debugf("Attempting to read config files under %q.", prowYAMLDirPath)
	if fileInfo, err := os.Stat(prowYAMLDirPath); !os.IsNotExist(err) && err == nil && fileInfo.IsDir() {
		mergeProwYAML := func(a, b *ProwYAML) *ProwYAML {
			c := &ProwYAML{}
			c.Presets = append(a.Presets, b.Presets...)
			c.Presubmits = append(a.Presubmits, b.Presubmits...)
			c.Postsubmits = append(a.Postsubmits, b.Postsubmits...)

			return c
		}

		err := filepath.Walk(prowYAMLDirPath, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && (filepath.Ext(p) == ".yaml" || filepath.Ext(p) == ".yml") {
				log.Debugf("Reading YAML file %q", p)
				bytes, err := ioutil.ReadFile(p)
				if err != nil {
					return err
				}
				partialProwYAML := &ProwYAML{}
				if err := yaml.Unmarshal(bytes, partialProwYAML); err != nil {
					return fmt.Errorf("failed to unmarshal %q: %w", p, err)
				}
				prowYAML = mergeProwYAML(prowYAML, partialProwYAML)
			}
			return err
		})
		if err != nil {
			return nil, fmt.Errorf("failed to read contents of directory %q: %w", inRepoConfigDirName, err)
		}
	} else {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading %q: %w", prowYAMLDirPath, err)
		}
		log.WithField("file", inRepoConfigFileName).Debug("Attempting to get inreconfigfile")
		prowYAMLFilePath := path.Join(repo.Directory(), inRepoConfigFileName)
		if _, err := os.Stat(prowYAMLFilePath); err == nil {
			bytes, err := ioutil.ReadFile(prowYAMLFilePath)
			if err != nil {
				return nil, fmt.Errorf("failed to read %q: %w", prowYAMLDirPath, err)
			}
			if err := yaml.Unmarshal(bytes, prowYAML); err != nil {
				return nil, fmt.Errorf("failed to unmarshal %q: %v", prowYAMLDirPath, err)
			}
		} else {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to check if file %q exists: %v", prowYAMLDirPath, err)
			}
		}
	}

	if err := DefaultAndValidateProwYAML(c, prowYAML, identifier); err != nil {
		return nil, err
	}

	log.Debugf("Successfully got %d presubmits and %d postsubmits from %q.", len(prowYAML.Presubmits), len(prowYAML.Postsubmits), inRepoConfigFileName)
	return prowYAML, nil
}

func DefaultAndValidateProwYAML(c *Config, p *ProwYAML, identifier string) error {
	if err := defaultPresubmits(p.Presubmits, p.Presets, c, identifier); err != nil {
		return err
	}
	if err := defaultPostsubmits(p.Postsubmits, p.Presets, c, identifier); err != nil {
		return err
	}
	if err := validatePresubmits(append(p.Presubmits, c.PresubmitsStatic[identifier]...), c.PodNamespace); err != nil {
		return err
	}
	if err := validatePostsubmits(append(p.Postsubmits, c.PostsubmitsStatic[identifier]...), c.PodNamespace); err != nil {
		return err
	}

	var errs []error
	for _, pre := range p.Presubmits {
		if !c.InRepoConfigAllowsCluster(pre.Cluster, identifier) {
			errs = append(errs, fmt.Errorf("cluster %q is not allowed for repository %q", pre.Cluster, identifier))
		}
	}
	for _, post := range p.Postsubmits {
		if !c.InRepoConfigAllowsCluster(post.Cluster, identifier) {
			errs = append(errs, fmt.Errorf("cluster %q is not allowed for repository %q", post.Cluster, identifier))
		}
	}

	return utilerrors.NewAggregate(errs)
}

// InRepoConfigGitCache is a wrapper around a git.ClientFactory that allows for
// threadsafe reuse of git.RepoClients when one already exists for the specified repo.
type InRepoConfigGitCache struct {
	git.ClientFactory
	cache map[string]*skipCleanRepoClient
	sync.RWMutex
}

func NewInRepoConfigGitCache(factory git.ClientFactory) git.ClientFactory {
	if factory == nil {
		// Don't wrap a nil git factory, keep it nil so that errors are handled properly.
		return nil
	}
	return &InRepoConfigGitCache{
		ClientFactory: factory,
		cache:         map[string]*skipCleanRepoClient{},
	}
}

func (c *InRepoConfigGitCache) ClientFor(org, repo string) (git.RepoClient, error) {
	key := fmt.Sprintf("%s/%s", org, repo)
	c.RLock()
	cached, err := func() (git.RepoClient, error) {
		defer c.RUnlock()

		if client, ok := c.cache[key]; ok {
			client.Lock()
			// Don't unlock the client unless we get an error or the consumer indicates they are done by Clean()ing.
			if err := client.Fetch(); err != nil {
				client.Unlock()
				return nil, err
			}
			return client, nil
		}
		return nil, nil
	}()
	if cached != nil || err != nil {
		return cached, err
	}

	// The repo client was not cached, create a new one.
	c.Lock()
	defer c.Unlock()
	coreClient, err := c.ClientFactory.ClientFor(org, repo)
	if err != nil {
		return nil, err
	}
	client := &skipCleanRepoClient{
		RepoClient: coreClient,
	}
	client.Lock()
	c.cache[key] = client
	return client, nil
}

var _ git.RepoClient = &skipCleanRepoClient{}

type skipCleanRepoClient struct {
	git.RepoClient
	sync.Mutex
}

func (rc *skipCleanRepoClient) Clean() error {
	// Skip cleaning and unlock to allow reuse as a cached entry.
	rc.Unlock()
	return nil
}
