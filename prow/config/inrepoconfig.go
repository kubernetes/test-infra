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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	gitignore "github.com/denormal/go-gitignore"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sirupsen/logrus"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	gerritsource "k8s.io/test-infra/prow/gerrit/source"

	"k8s.io/test-infra/prow/git/types"
	"k8s.io/test-infra/prow/git/v2"
	"sigs.k8s.io/yaml"
)

const (
	inRepoConfigFileName = ".prow.yaml"
	inRepoConfigDirName  = ".prow"
)

var inrepoconfigRepoOpts = git.RepoOpts{
	// Technically we only need inRepoConfigDirName (".prow") because the
	// default "cone mode" of sparse checkouts already include files at the
	// toplevel (which would include ".prow.yaml").
	SparseCheckoutDirs: []string{inRepoConfigDirName},
	// The sparse checkout would avoid creating another copy of Git objects
	// from the mirror clone into the secondary clone.
	ShareObjectsWithPrimaryClone: true,
}

var inrepoconfigMetrics = struct {
	gitCloneDuration *prometheus.HistogramVec
	gitOtherDuration *prometheus.HistogramVec
}{
	gitCloneDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "inrepoconfig_git_client_acquisition_duration",
		Help:    "Seconds taken for acquiring a git client (may include an initial clone operation).",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 30, 45, 60, 90, 120, 180, 300, 600, 1200},
	}, []string{
		"org",
		"repo",
	}),
	gitOtherDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "inrepoconfig_git_other_duration",
		Help:    "Seconds taken after acquiring a git client and performing all other git operations (to read the ProwYAML of the repo).",
		Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 20, 30, 45, 60, 90, 120, 180, 300, 600},
	}, []string{
		"org",
		"repo",
	}),
}

func init() {
	prometheus.MustRegister(inrepoconfigMetrics.gitCloneDuration)
	prometheus.MustRegister(inrepoconfigMetrics.gitOtherDuration)
}

// +k8s:deepcopy-gen=true

// ProwYAML represents the content of a .prow.yaml file
// used to version Presubmits and Postsubmits inside the tested repo.
type ProwYAML struct {
	Presets     []Preset     `json:"presets"`
	Presubmits  []Presubmit  `json:"presubmits"`
	Postsubmits []Postsubmit `json:"postsubmits"`

	// ProwIgnored is a well known, unparsed field where non-Prow fields can
	// be defined without conflicting with unknown field validation.
	ProwIgnored *json.RawMessage `json:"prow_ignored,omitempty"`
}

// ProwYAMLGetter is used to retrieve a ProwYAML. Tests should provide
// their own implementation and set that on the Config.
type ProwYAMLGetter func(c *Config, gc git.ClientFactory, identifier, baseSHA string, headSHAs ...string) (*ProwYAML, error)

// Verify prowYAMLGetterWithDefaults and prowYAMLGetter are both of type
// ProwYAMLGetter.
var _ ProwYAMLGetter = prowYAMLGetterWithDefaults
var _ ProwYAMLGetter = prowYAMLGetter

// InRepoConfigGetter defines a common interface that both the Moonraker client
// and raw InRepoConfigCache can implement. This way, Prow components like Sub
// and Gerrit can choose either one (based on runtime flags), but regardless of
// the choice the surrounding code can still just call this GetProwYAML()
// interface method (without being aware whether the underlying implementation
// is going over the network to Moonraker or is done locally with the local
// InRepoConfigCache (LRU cache)).
type InRepoConfigGetter interface {
	GetInRepoConfig(identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) (*ProwYAML, error)
	GetPresubmits(identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Presubmit, error)
	GetPostsubmits(identifier string, baseSHAGetter RefGetter, headSHAGetters ...RefGetter) ([]Postsubmit, error)
}

// prowYAMLGetter is like prowYAMLGetterWithDefaults, but without default values
// (it does not call DefaultAndValidateProwYAML()). Its sole purpose is to allow
// caching of ProwYAMLs that are retrieved purely from the inrepoconfig's repo,
// __without__ having the contents modified by the main Config's own settings
// (which happens mostly inside DefaultAndValidateProwYAML()). prowYAMLGetter is
// only used by cache.GetPresubmits() and cache.GetPostsubmits().
func prowYAMLGetter(
	c *Config,
	gc git.ClientFactory,
	identifier string,
	baseSHA string,
	headSHAs ...string) (*ProwYAML, error) {

	log := logrus.WithField("repo", identifier)

	if gc == nil {
		log.Error("prowYAMLGetter was called with a nil git client")
		return nil, errors.New("gitClient is nil")
	}

	orgRepo := *NewOrgRepo(identifier)
	if orgRepo.Repo == "" {
		return nil, fmt.Errorf("didn't get two results when splitting repo identifier %q", identifier)
	}

	timeBeforeClone := time.Now()
	repoOpts := inrepoconfigRepoOpts
	// For Gerrit, the baseSHA could appear as a headSHA for postsubmits if the
	// change was a fast-forward merge. So we need to dedupe it with sets.
	repoOpts.NeededCommits = sets.New(baseSHA)
	repoOpts.NeededCommits.Insert(headSHAs...)
	repo, err := gc.ClientForWithRepoOpts(orgRepo.Org, orgRepo.Repo, repoOpts)
	inrepoconfigMetrics.gitCloneDuration.WithLabelValues(orgRepo.Org, orgRepo.Repo).Observe((float64(time.Since(timeBeforeClone).Seconds())))
	if err != nil {
		return nil, fmt.Errorf("failed to clone repo for %q: %w", identifier, err)
	}
	timeAfterClone := time.Now()
	defer func() {
		if err := repo.Clean(); err != nil {
			log.WithError(err).Error("Failed to clean up repo.")
		}
		inrepoconfigMetrics.gitOtherDuration.WithLabelValues(orgRepo.Org, orgRepo.Repo).Observe((float64(time.Since(timeAfterClone).Seconds())))
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

	// TODO(mpherman): This is to hopefully mittigate issue with gerrit merges. Need to come up with a solution that checks
	// each CLs merge strategy as they can differ. ifNecessary is just the gerrit default
	var mergeMethod types.PullRequestMergeType
	if gerritsource.IsGerritOrg(identifier) {
		mergeMethod = types.MergeIfNecessary
	} else {
		mergeMethod = c.Tide.MergeMethod(orgRepo)
	}

	log.WithField("merge-strategy", mergeMethod).Debug("Using merge strategy.")
	if err := repo.MergeAndCheckout(baseSHA, string(mergeMethod), headSHAs...); err != nil {
		return nil, fmt.Errorf("failed to merge: %w", err)
	}

	return ReadProwYAML(log, repo.Directory(), false)
}

// ReadProwYAML parses the .prow.yaml file or .prow directory, no commit checkout or defaulting is included.
func ReadProwYAML(log *logrus.Entry, dir string, strict bool) (*ProwYAML, error) {
	prowYAML := &ProwYAML{}
	var opts []yaml.JSONOpt
	if strict {
		opts = append(opts, yaml.DisallowUnknownFields)
	}

	prowYAMLDirPath := path.Join(dir, inRepoConfigDirName)
	log.Debugf("Attempting to read config files under %q.", prowYAMLDirPath)
	if fileInfo, err := os.Stat(prowYAMLDirPath); !os.IsNotExist(err) && err == nil && fileInfo.IsDir() {
		mergeProwYAML := func(a, b *ProwYAML) *ProwYAML {
			c := &ProwYAML{}
			c.Presets = append(a.Presets, b.Presets...)
			c.Presubmits = append(a.Presubmits, b.Presubmits...)
			c.Postsubmits = append(a.Postsubmits, b.Postsubmits...)

			return c
		}
		prowIgnore, err := gitignore.NewRepositoryWithFile(dir, ProwIgnoreFileName)
		if err != nil {
			return nil, fmt.Errorf("failed to create `%s` parser: %w", ProwIgnoreFileName, err)
		}
		err = filepath.Walk(prowYAMLDirPath, func(p string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && (filepath.Ext(p) == ".yaml" || filepath.Ext(p) == ".yml") {
				// Use 'Match' directly because 'Ignore' and 'Include' don't work properly for repositories.
				match := prowIgnore.Match(p)
				if match != nil && match.Ignore() {
					return nil
				}
				log.Debugf("Reading YAML file %q", p)
				bytes, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				partialProwYAML := &ProwYAML{}
				if err := yaml.Unmarshal(bytes, partialProwYAML, opts...); err != nil {
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
		prowYAMLFilePath := path.Join(dir, inRepoConfigFileName)
		if _, err := os.Stat(prowYAMLFilePath); err == nil {
			bytes, err := os.ReadFile(prowYAMLFilePath)
			if err != nil {
				return nil, fmt.Errorf("failed to read %q: %w", prowYAMLDirPath, err)
			}
			if err := yaml.Unmarshal(bytes, prowYAML, opts...); err != nil {
				return nil, fmt.Errorf("failed to unmarshal %q: %w", prowYAMLDirPath, err)
			}
		} else {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to check if file %q exists: %w", prowYAMLDirPath, err)
			}
		}
	}
	return prowYAML, nil
}

// prowYAMLGetterWithDefaults is like prowYAMLGetter, but additionally sets
// defaults by calling DefaultAndValidateProwYAML.
func prowYAMLGetterWithDefaults(
	c *Config,
	gc git.ClientFactory,
	identifier string,
	baseSHA string,
	headSHAs ...string) (*ProwYAML, error) {

	prowYAML, err := prowYAMLGetter(c, gc, identifier, baseSHA, headSHAs...)
	if err != nil {
		return nil, err
	}

	// Mutate prowYAML to default values as necessary.
	if err := DefaultAndValidateProwYAML(c, prowYAML, identifier); err != nil {
		return nil, err
	}

	return prowYAML, nil
}

func DefaultAndValidateProwYAML(c *Config, p *ProwYAML, identifier string) error {
	if err := defaultPresubmits(p.Presubmits, p.Presets, c, identifier); err != nil {
		return err
	}
	if err := defaultPostsubmits(p.Postsubmits, p.Presets, c, identifier); err != nil {
		return err
	}
	if err := c.validatePresubmits(append(p.Presubmits, c.GetPresubmitsStatic(identifier)...)); err != nil {
		return err
	}
	if err := c.validatePostsubmits(append(p.Postsubmits, c.GetPostsubmitsStatic(identifier)...)); err != nil {
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

	if len(errs) == 0 {
		log := logrus.WithField("repo", identifier)
		log.Debugf("Successfully got %d presubmits and %d postsubmits.", len(p.Presubmits), len(p.Postsubmits))
	}

	return utilerrors.NewAggregate(errs)
}

// ContainsInRepoConfigPath indicates whether the specified list of changed
// files (repo relative paths) includes a file that might be an inrepo config file.
//
// This function could report a false positive as it doesn't consider .prowignore files.
// It is designed to be used to help short circuit when we know a change doesn't touch
// the inrepo config.
func ContainsInRepoConfigPath(files []string) bool {
	for _, file := range files {
		if file == inRepoConfigFileName {
			return true
		}
		if strings.HasPrefix(file, inRepoConfigDirName+"/") {
			return true
		}
	}
	return false
}
