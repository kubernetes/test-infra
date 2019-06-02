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

package api

import (
	"fmt"
	"io/ioutil"
	"os"

	"github.com/sirupsen/logrus"
	"sigs.k8s.io/yaml"

	checkconfigapi "k8s.io/test-infra/prow/cmd/checkconfig/api"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
)

const (
	// PluginName is the name of this plugin
	PluginName = "inrepoconfig"

	// ContextName is the name of the context the inrepoconfig plugin creates at GitHub
	ContextName = "prow-config-parsing"

	// ConfigFileName is the name of the configfile the inrepoconfig plugin uses to read its
	// config from
	ConfigFileName = "prow.yaml"
)

// InRepoConfig is the type `prow.yaml` gets deserialized into
type InRepoConfig struct {
	Presubmits []config.Presubmit `json:"presubmits,omitempty"`
}

// defaultAndValidateInRepoConfig defaults and validates the InRepoConfig. This must be called before
// accessing any data in it
func (irc *InRepoConfig) defaultAndValidateInRepoConfig(c *config.Config, orgRepo string) error {
	for i := range irc.Presubmits {
		ps := &irc.Presubmits[i]
		config.DefaultPresubmitFields(&c.ProwConfig, ps)
		if ps.Decorate {
			ps.DecorationConfig = ps.DecorationConfig.ApplyDefault(c.Plank.DefaultDecorationConfig)
		}
		if err := config.ResolvePresets(
			ps.Name, ps.Labels, ps.Spec, ps.BuildSpec, c.Presets); err != nil {
			return err
		}
	}

	// Has to be done in the end, otherwise the jobs Trigger field is unset
	if err := config.SetPresubmitRegexes(irc.Presubmits); err != nil {
		return fmt.Errorf("failed to set Presubmit regexes: %v", err)
	}

	for _, presubmit := range irc.Presubmits {
		if err := checkconfigapi.ValidatePresubmitJob(orgRepo, presubmit); err != nil {
			return err
		}
	}
	ap := &additionalPresubmits{
		fullRepoName: orgRepo,
		presubmits:   irc.Presubmits,
	}
	// This is not extremely efficient as it validates all Job types on all Repos of this
	// Prow instance.
	// Changing that requires a bigger refactoring of config.Config.ValidateJobConfig thought,
	// in order to keep the change required when introducing `inrepoconfig` optimizing this is
	// kept as a future task.
	// TODO @alvaroaleman: Revisit and improve this
	if err := c.ValidateJobConfig(ap); err != nil {
		return err
	}

	return nil
}

// New returns an InRepoConfig for the passed in refs
func New(log *logrus.Entry, c *config.Config, gc *git.Client, org, repo, baseRef string, headRefs []string) (*InRepoConfig, error) {
	mergeStrategyRaw := ""
	githubMergeMethod := c.Tide.MergeMethod(org, repo)
	switch githubMergeMethod {
	case github.MergeRebase:
		return nil, fmt.Errorf("merge strategy %s is currently not supported by %s", githubMergeMethod, PluginName)
	case github.MergeMerge:
		mergeStrategyRaw = "--no-ff"
	case github.MergeSquash:
		mergeStrategyRaw = "--squash"
	}

	clonedRepo, err := gc.Clone(org + "/" + repo)
	if err != nil {
		return nil, fmt.Errorf("failed to clone repo %s/%s: %v", org, repo, err)
	}
	defer func() {
		if err := clonedRepo.Clean(); err != nil {
			log.WithError(err).Errorf("failed to clean up cloned repo %s/%s", org, repo)
		}
	}()

	if err := clonedRepo.CheckoutMergedPullRequests(baseRef, headRefs, mergeStrategyRaw); err != nil {
		return nil, fmt.Errorf("failed to checkout pull request: %v", err)
	}

	configFile := fmt.Sprintf("%s/%s", clonedRepo.Dir, ConfigFileName)
	if _, err := os.Stat(configFile); err != nil {
		if os.IsNotExist(err) {
			return &InRepoConfig{}, nil
		}
		return nil, fmt.Errorf("failed to check if %q exists: %v", ConfigFileName, err)
	}

	configRaw, err := ioutil.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read %q: %v", ConfigFileName, err)
	}

	irc := &InRepoConfig{}
	if err := yaml.UnmarshalStrict(configRaw, irc); err != nil {
		return nil, fmt.Errorf("failed to parse %q: %v", ConfigFileName, err)
	}

	if err := irc.defaultAndValidateInRepoConfig(c, org+"/"+repo); err != nil {
		return nil, err
	}

	return irc, nil
}

type additionalPresubmits struct {
	fullRepoName string
	presubmits   []config.Presubmit
}

func (ap *additionalPresubmits) Presubmits() (string, []config.Presubmit) {
	return ap.fullRepoName, ap.presubmits
}
