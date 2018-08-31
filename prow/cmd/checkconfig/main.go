/*
Copyright 2018 The Kubernetes Authors.

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

// checkconfig loads configuration for Prow to validate it
package main

import (
	"errors"
	"flag"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/errorutil"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/plugins/approve"

	"k8s.io/test-infra/prow/config"
	_ "k8s.io/test-infra/prow/hook"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/lgtm"
)

type options struct {
	configPath    string
	jobConfigPath string
	pluginConfig  string

	warnings flagutil.Strings
	strict   bool
}

func (o *options) reportWarning(err error, msg string) {
	if o.strict {
		logrus.WithError(err).Fatal(msg)
	}
	logrus.WithError(err).Warn(msg)
}

func (o *options) warningEnabled(warning string) bool {
	for _, registeredWarning := range o.warnings.Strings() {
		if warning == registeredWarning {
			return true
		}
	}
	return false
}

const (
	mismatchedTideWarning   = "mismatched-tide"
	nonDecoratedJobsWarning = "non-decorated-jobs"
)

var allWarnings = []string{
	mismatchedTideWarning,
	nonDecoratedJobsWarning,
}

func (o *options) Validate() error {
	if o.configPath == "" {
		return errors.New("required flag --config-path was unset")
	}
	if o.pluginConfig == "" {
		return errors.New("required flag --plugin-config was unset")
	}
	for _, warning := range o.warnings.Strings() {
		found := false
		for _, registeredWarning := range allWarnings {
			if warning == registeredWarning {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("no such warning %q, valid warnings: %v", warning, allWarnings)
		}
	}
	return nil
}

func gatherOptions() options {
	o := options{}
	flag.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	flag.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	flag.StringVar(&o.pluginConfig, "plugin-config", "", "Path to plugin config file.")
	flag.Var(&o.warnings, "warnings", "Comma-delimited list of warnings to validate.")
	flag.BoolVar(&o.strict, "strict", false, "If set, consider all warnings as errors.")
	flag.Parse()
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	// use all warnings by default
	if len(o.warnings.Strings()) == 0 {
		o.warnings = flagutil.NewStrings(allWarnings...)
	}

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "checkconfig"}),
	)

	configAgent := config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error loading Prow config.")
	}

	pluginAgent := plugins.PluginAgent{}
	if err := pluginAgent.Load(o.pluginConfig); err != nil {
		logrus.WithError(err).Fatal("Error loading Prow plugin config.")
	}

	// the following checks are useful in finding user errors but their
	// presence won't lead to strictly incorrect behavior, so we can
	// detect them here but don't necessarily want to stop config re-load
	// in all components on their failure.
	if o.warningEnabled(mismatchedTideWarning) {
		if err := validateTideRequirements(configAgent, pluginAgent); err != nil {
			o.reportWarning(err, "Invalid tide merge configuration.")
		}
	}
	if o.warningEnabled(nonDecoratedJobsWarning) {
		if err := validateDecoratedJobs(configAgent); err != nil {
			o.reportWarning(err, "Invalid tide merge configuration.")
		}
	}
}

func validateTideRequirements(configAgent config.Agent, pluginAgent plugins.PluginAgent) error {
	lgtmPluginConfig := newOrgRepoConfig(pluginAgent.Config().EnabledReposForPlugin(lgtm.PluginName))
	approvePluginConfig := newOrgRepoConfig(pluginAgent.Config().EnabledReposForPlugin(approve.PluginName))

	lgtmTideConfig, approveTideConfig, overallTideConfig := newOrgRepoConfig([]string{}, []string{}), newOrgRepoConfig([]string{}, []string{}), newOrgRepoConfig([]string{}, []string{})
	for _, query := range configAgent.Config().Tide.Queries {
		requiresLgtm := sets.NewString(query.Labels...).Has(lgtm.LgtmLabel)
		requiresApproval := sets.NewString(query.Labels...).Has(approve.ApprovedLabel)
		for _, org := range query.Orgs {
			if requiresLgtm {
				lgtmTideConfig.orgs.Insert(org)
			}
			if requiresApproval {
				approveTideConfig.orgs.Insert(org)
			}
			overallTideConfig.orgs.Insert(org)
		}
		for _, repo := range query.Repos {
			if requiresLgtm {
				lgtmTideConfig.repos.Insert(repo)
			}
			if requiresApproval {
				approveTideConfig.repos.Insert(repo)
			}
			overallTideConfig.repos.Insert(repo)
		}
	}

	var validationErrs []error
	validationErrs = append(validationErrs, ensureValidConfiguration(lgtm.PluginName, lgtm.LgtmLabel, lgtmTideConfig, overallTideConfig, lgtmPluginConfig))
	validationErrs = append(validationErrs, ensureValidConfiguration(approve.PluginName, approve.ApprovedLabel, approveTideConfig, overallTideConfig, approvePluginConfig))

	return errorutil.NewAggregate(validationErrs...)
}

func newOrgRepoConfig(orgs, repos []string) orgRepoConfig {
	return orgRepoConfig{
		orgs:  sets.NewString(orgs...),
		repos: sets.NewString(repos...),
	}
}

type orgRepoConfig struct {
	orgs, repos sets.String
}

func (c *orgRepoConfig) items() []string {
	return c.orgs.Union(c.repos).UnsortedList()
}

func (c *orgRepoConfig) has(target string) bool {
	return c.repos.Has(target) || c.orgs.Has(target) || (strings.Contains(target, "/") && c.orgs.Has(strings.Split(target, "/")[0]))
}

// ensureValidConfiguration enforces rules about tide and plugin config.
// In this context, a subset is the set of repos or orgs for which a specific
// plugin is either enabled (for plugins) or required for merge (for tide). The
// tide superset is every org or repo that has any configuration at all in tide.
// Specifically:
//   - every item in the tide subset must also be in the plugins subset
//   - every item in the plugins subset that is in the tide superset must also be in the tide subset
// For example:
//   - if org/repo is configured in tide to require lgtm, it must have the lgtm plugin enabled
//   - if org/repo is configured in tide, the tide configuration must require the same set of
//     plugins as are configured. If the repository has LGTM and approve enabled, the tide query
//     must require both labels
func ensureValidConfiguration(plugin, label string, tideSubSet, tideSuperSet, pluginsSubSet orgRepoConfig) error {
	var notEnabled []string
	for _, target := range tideSubSet.items() {
		if !pluginsSubSet.has(target) {
			notEnabled = append(notEnabled, target)
		}
	}

	var tideConfigured []string
	for _, target := range pluginsSubSet.items() {
		if tideSuperSet.has(target) {
			tideConfigured = append(tideConfigured, target)
		}
	}

	var notRequired []string
	for _, target := range tideConfigured {
		if !tideSubSet.has(target) {
			notRequired = append(notRequired, target)
		}
	}

	var configErrors []error
	if len(notEnabled) > 0 {
		configErrors = append(configErrors, fmt.Errorf("the following orgs or repos require %s for merging but do not enable the %s plugin: %v", label, plugin, notEnabled))
	}
	if len(notRequired) > 0 {
		configErrors = append(configErrors, fmt.Errorf("the following orgs or repos enables the %s plugin but do not require the %s label for merging: %v", plugin, label, notRequired))
	}

	return errorutil.NewAggregate(configErrors...)
}

func validateDecoratedJobs(configAgent config.Agent) error {
	var nonDecoratedJobs []string
	for _, presubmit := range configAgent.Config().AllPresubmits([]string{}) {
		if presubmit.Agent == string(v1.KubernetesAgent) && !presubmit.Decorate {
			nonDecoratedJobs = append(nonDecoratedJobs, presubmit.Name)
		}
	}

	for _, postsubmit := range configAgent.Config().AllPostsubmits([]string{}) {
		if postsubmit.Agent == string(v1.KubernetesAgent) && !postsubmit.Decorate {
			nonDecoratedJobs = append(nonDecoratedJobs, postsubmit.Name)
		}
	}

	for _, periodic := range configAgent.Config().AllPeriodics() {
		if periodic.Agent == string(v1.KubernetesAgent) && !periodic.Decorate {
			nonDecoratedJobs = append(nonDecoratedJobs, periodic.Name)
		}
	}

	if len(nonDecoratedJobs) > 0 {
		return fmt.Errorf("the following jobs use the kubernetes provider but do not use the pod utilites: %v", nonDecoratedJobs)
	}
	return nil
}
