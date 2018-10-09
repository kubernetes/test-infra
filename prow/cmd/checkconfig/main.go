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
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/errorutil"
	needsrebase "k8s.io/test-infra/prow/external-plugins/needs-rebase/plugin"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/plugins/approve"
	"k8s.io/test-infra/prow/plugins/blockade"
	"k8s.io/test-infra/prow/plugins/cherrypickunapproved"
	"k8s.io/test-infra/prow/plugins/hold"
	"k8s.io/test-infra/prow/plugins/releasenote"
	"k8s.io/test-infra/prow/plugins/trigger"
	"k8s.io/test-infra/prow/plugins/verify-owners"
	"k8s.io/test-infra/prow/plugins/wip"

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

func reportWarning(strict bool, errs errorutil.Aggregate) {
	for _, item := range errs.Strings() {
		logrus.Warn(item)
	}
	if strict {
		logrus.Fatal("Strict is set and there were warnings")
	}
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
	jobNameLengthWarning    = "long-job-names"
	needsOkToTestWarning    = "needs-ok-to-test"
)

var allWarnings = []string{
	mismatchedTideWarning,
	nonDecoratedJobsWarning,
	jobNameLengthWarning,
	needsOkToTestWarning,
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
		logrusutil.NewDefaultFieldsFormatter(&logrus.TextFormatter{}, logrus.Fields{"component": "checkconfig"}),
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
	var errs []error
	if o.warningEnabled(mismatchedTideWarning) {
		if err := validateTideRequirements(configAgent, pluginAgent); err != nil {
			errs = append(errs, err)
		}
	}
	if o.warningEnabled(nonDecoratedJobsWarning) {
		if err := validateDecoratedJobs(configAgent); err != nil {
			errs = append(errs, err)
		}
	}
	if o.warningEnabled(jobNameLengthWarning) {
		if err := validateJobRequirements(configAgent); err != nil {
			errs = append(errs, err)
		}
	}
	if o.warningEnabled(needsOkToTestWarning) {
		if err := validateNeedsOkToTestLabel(configAgent); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		reportWarning(o.strict, errorutil.NewAggregate(errs...))
	}
}

func validateJobRequirements(configAgent config.Agent) error {
	c := configAgent.Config().JobConfig

	var validationErrs []error
	for repo, jobs := range c.Presubmits {
		for _, job := range jobs {
			validationErrs = append(validationErrs, validatePresubmitJob(repo, job))
		}
	}
	for repo, jobs := range c.Postsubmits {
		for _, job := range jobs {
			validationErrs = append(validationErrs, validatePostsubmitJob(repo, job))
		}
	}
	for _, job := range c.Periodics {
		validationErrs = append(validationErrs, validatePeriodicJob(job))
	}

	return errorutil.NewAggregate(validationErrs...)
}

func validatePresubmitJob(repo string, job config.Presubmit) error {
	var validationErrs []error
	// Prow labels k8s resources with job names. Labels are capped at 63 chars.
	if job.Agent == string(v1.KubernetesAgent) && len(job.Name) > validation.LabelValueMaxLength {
		validationErrs = append(validationErrs, fmt.Errorf("name of Presubmit job %q (for repo %q) too long (should be at most 63 characters)", job.Name, repo))
	}
	return errorutil.NewAggregate(validationErrs...)
}

func validatePostsubmitJob(repo string, job config.Postsubmit) error {
	var validationErrs []error
	// Prow labels k8s resources with job names. Labels are capped at 63 chars.
	if job.Agent == string(v1.KubernetesAgent) && len(job.Name) > validation.LabelValueMaxLength {
		validationErrs = append(validationErrs, fmt.Errorf("name of Postsubmit job %q (for repo %q) too long (should be at most 63 characters)", job.Name, repo))
	}
	return errorutil.NewAggregate(validationErrs...)
}

func validatePeriodicJob(job config.Periodic) error {
	var validationErrs []error
	// Prow labels k8s resources with job names. Labels are capped at 63 chars.
	if job.Agent == string(v1.KubernetesAgent) && len(job.Name) > validation.LabelValueMaxLength {
		validationErrs = append(validationErrs, fmt.Errorf("name of Periodic job %q too long (should be at most 63 characters)", job.Name))
	}
	return errorutil.NewAggregate(validationErrs...)
}

func validateTideRequirements(configAgent config.Agent, pluginAgent plugins.PluginAgent) error {
	type matcher struct {
		// matches determines if the tide query appropriately honors the
		// label in question -- whether by requiring it or forbidding it
		matches func(label string, query config.TideQuery) bool
		// verb is used in forming error messages
		verb string
	}
	requires := matcher{
		matches: func(label string, query config.TideQuery) bool {
			return sets.NewString(query.Labels...).Has(label)
		},
		verb: "require",
	}
	forbids := matcher{
		matches: func(label string, query config.TideQuery) bool {
			return sets.NewString(query.MissingLabels...).Has(label)
		},
		verb: "forbid",
	}

	// configs list relationships between tide config
	// and plugin enablement that we want to validate
	configs := []struct {
		// plugin and label identify the relationship we are validating
		plugin, label string
		// matcher determines if the tide query appropriately honors the
		// label in question -- whether by requiring it or forbidding it
		matcher matcher
		// config holds the orgs and repos for which tide does honor the
		// label; this container is populated conditionally from queries
		// using the matcher
		config orgRepoConfig
	}{
		{plugin: lgtm.PluginName, label: lgtm.LGTMLabel, matcher: requires},
		{plugin: approve.PluginName, label: approve.ApprovedLabel, matcher: requires},
		{plugin: hold.PluginName, label: hold.Label, matcher: forbids},
		{plugin: wip.PluginName, label: wip.Label, matcher: forbids},
		{plugin: verifyowners.PluginName, label: verifyowners.InvalidOwnersLabel, matcher: forbids},
		{plugin: releasenote.PluginName, label: releasenote.ReleaseNoteLabelNeeded, matcher: forbids},
		{plugin: cherrypickunapproved.PluginName, label: cherrypickunapproved.CpUnapprovedLabel, matcher: forbids},
		{plugin: blockade.PluginName, label: blockade.BlockedPathsLabel, matcher: forbids},
		{plugin: needsrebase.PluginName, label: needsrebase.NeedsRebaseLabel, matcher: forbids},
	}

	for i := range configs {
		configs[i].config = newOrgRepoConfig([]string{}, []string{})
		for _, query := range configAgent.Config().Tide.Queries {
			if configs[i].matcher.matches(configs[i].label, query) {
				for _, org := range query.Orgs {
					configs[i].config.orgs.Insert(org)
				}
				for _, repo := range query.Repos {
					configs[i].config.repos.Insert(repo)
				}
			}
		}
	}

	overallTideConfig := newOrgRepoConfig([]string{}, []string{})
	for _, query := range configAgent.Config().Tide.Queries {
		for _, org := range query.Orgs {
			overallTideConfig.orgs.Insert(org)
		}
		for _, repo := range query.Repos {
			overallTideConfig.repos.Insert(repo)
		}
	}

	var validationErrs []error
	for _, pluginConfig := range configs {
		validationErrs = append(validationErrs, ensureValidConfiguration(
			pluginConfig.plugin, pluginConfig.label, pluginConfig.matcher.verb, pluginConfig.config, overallTideConfig,
			newOrgRepoConfig(pluginAgent.Config().EnabledReposForPlugin(pluginConfig.plugin))),
		)
	}

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
func ensureValidConfiguration(plugin, label, verb string, tideSubSet, tideSuperSet, pluginsSubSet orgRepoConfig) error {
	var notEnabled []string
	for _, target := range tideSubSet.items() {
		if !pluginsSubSet.has(target) {
			notEnabled = append(notEnabled, target)
		}
	}

	var notRequired []string
	for _, target := range pluginsSubSet.items() {
		if tideSuperSet.has(target) && !tideSubSet.has(target) {
			notRequired = append(notRequired, target)
		}
	}

	var configErrors []error
	if len(notEnabled) > 0 {
		configErrors = append(configErrors, fmt.Errorf("the following orgs or repos %s the %s label for merging but do not enable the %s plugin: %v", verb, label, plugin, notEnabled))
	}
	if len(notRequired) > 0 {
		configErrors = append(configErrors, fmt.Errorf("the following orgs or repos enable the %s plugin but do not %s the %s label for merging: %v", plugin, verb, label, notRequired))
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
		return fmt.Errorf("the following jobs use the kubernetes provider but do not use the pod utilities: %v", nonDecoratedJobs)
	}
	return nil
}

func validateNeedsOkToTestLabel(configAgent config.Agent) error {
	var queryErrors []error
	for i, query := range configAgent.Config().Tide.Queries {
		for _, label := range query.Labels {
			if label == lgtm.LGTMLabel {
				for _, label := range query.MissingLabels {
					if label == trigger.NeedsOkToTest {
						queryErrors = append(queryErrors, fmt.Errorf("the tide query at position %d forbids the %q label and requires the %q label, which is not recommended; see https://github.com/kubernetes/test-infra/blob/master/prow/cmd/tide/maintainers.md#best-practices for more information", i, trigger.NeedsOkToTest, lgtm.LGTMLabel))
					}
				}
			}
		}
	}
	return errorutil.NewAggregate(queryErrors...)
}
