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
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	stdio "io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"

	utilerrors "k8s.io/apimachinery/pkg/util/errors"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	needsrebase "k8s.io/test-infra/prow/external-plugins/needs-rebase/plugin"
	"k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	pluginsflagutil "k8s.io/test-infra/prow/flagutil/plugins"
	"k8s.io/test-infra/prow/github"
	_ "k8s.io/test-infra/prow/hook/plugin-imports"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/plank"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/approve"
	"k8s.io/test-infra/prow/plugins/blockade"
	"k8s.io/test-infra/prow/plugins/blunderbuss"
	"k8s.io/test-infra/prow/plugins/bugzilla"
	"k8s.io/test-infra/prow/plugins/cherrypickunapproved"
	"k8s.io/test-infra/prow/plugins/hold"
	labelplugin "k8s.io/test-infra/prow/plugins/label"
	"k8s.io/test-infra/prow/plugins/lgtm"
	ownerslabel "k8s.io/test-infra/prow/plugins/owners-label"
	"k8s.io/test-infra/prow/plugins/releasenote"
	"k8s.io/test-infra/prow/plugins/trigger"
	verifyowners "k8s.io/test-infra/prow/plugins/verify-owners"
	"k8s.io/test-infra/prow/plugins/wip"
)

type options struct {
	config        configflagutil.ConfigOptions
	pluginsConfig pluginsflagutil.PluginOptions

	prowYAMLRepoName string
	prowYAMLPath     string

	warnings               flagutil.Strings
	excludeWarnings        flagutil.Strings
	requiredJobAnnotations flagutil.Strings
	strict                 bool
	expensive              bool
	includeDefaultWarnings bool

	github  flagutil.GitHubOptions
	storage flagutil.StorageClientOptions
}

func reportWarning(strict bool, errs utilerrors.Aggregate) {
	for _, item := range errs.Errors() {
		logrus.Warn(item.Error())
	}
	if strict {
		logrus.Fatal("Strict is set and there were warnings")
	}
}

func (o *options) warningEnabled(warning string) bool {
	return sets.New[string](o.warnings.Strings()...).Difference(sets.New[string](o.excludeWarnings.Strings()...)).Has(warning)
}

const (
	mismatchedTideWarning                         = "mismatched-tide"
	mismatchedTideLenientWarning                  = "mismatched-tide-lenient"
	tideStrictBranchWarning                       = "tide-strict-branch"
	tideContextPolicy                             = "tide-context-policy"
	nonDecoratedJobsWarning                       = "non-decorated-jobs"
	validDecorationConfigWarning                  = "valid-decoration-config"
	jobNameLengthWarning                          = "long-job-names"
	jobRefsDuplicationWarning                     = "duplicate-job-refs"
	needsOkToTestWarning                          = "needs-ok-to-test"
	managedWebhooksWarning                        = "managed-webhooks"
	validateOwnersWarning                         = "validate-owners"
	missingTriggerWarning                         = "missing-trigger"
	validateURLsWarning                           = "validate-urls"
	unknownFieldsWarning                          = "unknown-fields"
	unknownFieldsAllWarning                       = "unknown-fields-all" // Superset of "unknown-fields" that includes validating job config.
	verifyOwnersFilePresence                      = "verify-owners-presence"
	validateClusterFieldWarning                   = "validate-cluster-field"
	validateSupplementalProwConfigOrgRepoHirarchy = "validate-supplemental-prow-config-hirarchy"
	validateUnmanagedBranchConfigHasNoSubconfig   = "validate-unmanaged-branchconfig-has-no-subconfig"
	validateGitHubAppInstallationWarning          = "validate-github-app-installation"
	validateLabelWarning                          = "validate-label"
	requiredJobAnnotationsWarning                 = "required-job-annotations"
	periodicDefaultCloneWarning                   = "periodic-default-clone-config"

	defaultHourlyTokens = 3000
	defaultAllowedBurst = 100
)

var defaultWarnings = []string{
	mismatchedTideWarning,
	tideStrictBranchWarning,
	tideContextPolicy,
	mismatchedTideLenientWarning,
	nonDecoratedJobsWarning,
	jobNameLengthWarning,
	jobRefsDuplicationWarning,
	needsOkToTestWarning,
	managedWebhooksWarning,
	validateOwnersWarning,
	missingTriggerWarning,
	validateURLsWarning,
	unknownFieldsWarning,
	validateClusterFieldWarning,
	validateSupplementalProwConfigOrgRepoHirarchy,
	validateUnmanagedBranchConfigHasNoSubconfig,
	validateLabelWarning,
	requiredJobAnnotationsWarning,
	periodicDefaultCloneWarning,
}

var expensiveWarnings = []string{
	verifyOwnersFilePresence,
}

var optionalWarnings = []string{
	validDecorationConfigWarning,
	// It would be nice to make "unknown-fields-all" a default, but difficult to do due to K8s configs.
	// https://github.com/kubernetes/test-infra/pull/21075#issuecomment-862550510
	unknownFieldsAllWarning,
	validateGitHubAppInstallationWarning,
}

var throttlerDefaults = flagutil.ThrottlerDefaults(defaultHourlyTokens, defaultAllowedBurst)

func getAllWarnings() []string {
	var all []string
	all = append(all, defaultWarnings...)
	all = append(all, expensiveWarnings...)
	all = append(all, optionalWarnings...)

	return all
}

func (o *options) DefaultAndValidate() error {
	allWarnings := getAllWarnings()
	for _, validate := range []interface{ Validate(bool) error }{&o.config, &o.pluginsConfig, &o.storage} {
		if err := validate.Validate(false); err != nil {
			return err
		}
	}

	if o.prowYAMLPath != "" && o.prowYAMLRepoName == "" {
		return errors.New("--prow-yaml-repo-path requires --prow-yaml-repo-name to be set")
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

func parseOptions() (options, error) {
	o := options{}

	if err := o.gatherOptions(flag.CommandLine, os.Args[1:]); err != nil {
		return options{}, err
	}
	return o, nil
}

func (o *options) gatherOptions(flag *flag.FlagSet, args []string) error {
	o.pluginsConfig.CheckUnknownPlugins = true
	flag.StringVar(&o.prowYAMLRepoName, "prow-yaml-repo-name", "", "Name of the repo whose .prow.yaml should be checked.")
	flag.StringVar(&o.prowYAMLPath, "prow-yaml-path", "", "Path to the .prow.yaml file to check. Requires --prow-yaml-repo-name to be set. Omit to look for either .prow.yaml or a .prow directory in the current working directory (recommended).")
	flag.Var(&o.warnings, "warnings", "Warnings to validate. Use repeatedly to provide a list of warnings")
	flag.Var(&o.excludeWarnings, "exclude-warning", "Warnings to exclude. Use repeatedly to provide a list of warnings to exclude")
	flag.Var(&o.requiredJobAnnotations, "required-job-annotations", "Required annotation names that job has to include in a definition. Use repeatedly to provide a list of required annotations")
	flag.BoolVar(&o.expensive, "expensive-checks", false, "If set, additional expensive warnings will be enabled")
	flag.BoolVar(&o.strict, "strict", false, "If set, consider all warnings as errors.")
	flag.BoolVar(&o.includeDefaultWarnings, "include-default-warnings", false, "If set force inclusion of default warning set. Normally this is inferred based on a lack of '--warnings' flags.")
	o.github.AddCustomizedFlags(flag, throttlerDefaults)
	o.github.AllowAnonymous = true
	o.config.AddFlags(flag)
	o.pluginsConfig.AddFlags(flag)
	o.storage.AddFlags(flag)
	if err := flag.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}
	if err := o.DefaultAndValidate(); err != nil {
		return fmt.Errorf("invalid options: %w", err)
	}
	return nil
}

func main() {
	logrusutil.ComponentInit()

	o, err := parseOptions()
	if err != nil {
		logrus.Fatalf("Error parsing options - %v", err)
	}

	if err := validate(o); err != nil {
		switch e := err.(type) {
		case utilerrors.Aggregate:
			reportWarning(o.strict, e)
		default:
			logrus.WithError(err).Fatal("Validation failed")
		}

	} else {
		logrus.Info("checkconfig passes without any error!")
	}
}

func validate(o options) error {
	// use all warnings by default
	if len(o.warnings.Strings()) == 0 || o.includeDefaultWarnings {
		if o.expensive {
			o.warnings = flagutil.NewStrings(append(o.warnings.Strings(), getAllWarnings()...)...)
		} else {
			o.warnings = flagutil.NewStrings(append(o.warnings.Strings(), defaultWarnings...)...)
		}
	}
	if o.github.AppID != "" && o.github.AppPrivateKeyPath != "" {
		o.warnings.Add(validateGitHubAppInstallationWarning)
	}

	configAgent, err := o.config.ConfigAgent()
	if err != nil {
		return fmt.Errorf("error loading prow config: %w", err)
	}
	cfg := configAgent.Config()

	if o.prowYAMLRepoName != "" {
		if err := validateInRepoConfig(cfg, o.prowYAMLPath, o.prowYAMLRepoName, o.warningEnabled(unknownFieldsAllWarning)); err != nil {
			return fmt.Errorf("error validating .prow.yaml: %w", err)
		}
	}

	var pcfg *plugins.Configuration
	if o.pluginsConfig.PluginConfigPath != "" {
		pluginAgent, err := o.pluginsConfig.PluginAgent()
		if err != nil {
			return fmt.Errorf("error loading Prow plugin config: %w", err)
		}
		pcfg = pluginAgent.Config()
	}

	// the following checks are useful in finding user errors but their
	// presence won't lead to strictly incorrect behavior, so we can
	// detect them here but don't necessarily want to stop config re-load
	// in all components on their failure.
	var errs []error
	if pcfg != nil && o.warningEnabled(verifyOwnersFilePresence) {
		if o.github.TokenPath == "" {
			return errors.New("cannot verify OWNERS file presence without a GitHub token")
		}

		githubClient, err := o.github.GitHubClient(false)
		if err != nil {
			return fmt.Errorf("error loading GitHub client: %w", err)
		}
		// 404s are expected to happen, no point in retrying
		githubClient.SetMax404Retries(0)

		if err := verifyOwnersPresence(pcfg, githubClient); err != nil {
			errs = append(errs, err)
		}
	}
	if pcfg != nil && o.warningEnabled(mismatchedTideWarning) {
		if err := validateTideRequirements(cfg, pcfg, true); err != nil {
			errs = append(errs, err)
		}
	} else if pcfg != nil && o.warningEnabled(mismatchedTideLenientWarning) {
		if err := validateTideRequirements(cfg, pcfg, false); err != nil {
			errs = append(errs, err)
		}
	}
	if o.warningEnabled(nonDecoratedJobsWarning) {
		if err := validateDecoratedJobs(cfg); err != nil {
			errs = append(errs, err)
		}
	}
	if o.warningEnabled(validDecorationConfigWarning) {
		if err := validateDecorationConfig(cfg); err != nil {
			errs = append(errs, err)
		}
	}
	if o.warningEnabled(jobNameLengthWarning) {
		if err := validateJobRequirements(cfg.JobConfig); err != nil {
			errs = append(errs, err)
		}
	}
	if o.warningEnabled(jobRefsDuplicationWarning) {
		if err := validateJobExtraRefs(cfg.JobConfig); err != nil {
			errs = append(errs, err)
		}
	}
	if o.warningEnabled(periodicDefaultCloneWarning) {
		if err := validatePeriodicDefaultCloneConfig(cfg.JobConfig); err != nil {
			errs = append(errs, err)
		}
	}
	if o.warningEnabled(needsOkToTestWarning) {
		if err := validateNeedsOkToTestLabel(cfg); err != nil {
			errs = append(errs, err)
		}
	}
	if o.warningEnabled(managedWebhooksWarning) {
		if err := validateManagedWebhooks(cfg); err != nil {
			errs = append(errs, err)
		}
	}
	if pcfg != nil && o.warningEnabled(validateOwnersWarning) {
		if err := verifyOwnersPlugin(pcfg); err != nil {
			errs = append(errs, err)
		}
	}
	if pcfg != nil && o.warningEnabled(missingTriggerWarning) {
		if err := validateTriggers(cfg, pcfg); err != nil {
			errs = append(errs, err)
		}
	}
	if pcfg != nil && o.warningEnabled(validateURLsWarning) {
		if err := validateURLs(cfg.ProwConfig); err != nil {
			errs = append(errs, err)
		}
	}
	// If both "unknown-fields" and "unknown-fields-all" are enabled, just run "unknown-fields-all" validation
	// since it is a superset. This will avoid duplicate warnings.
	unknownAllEnabled := o.warningEnabled(unknownFieldsAllWarning)
	unknownEnabled := o.warningEnabled(unknownFieldsWarning)
	if unknownAllEnabled {
		if _, err := config.LoadStrict(o.config.ConfigPath, o.config.JobConfigPath, nil, ""); err != nil {
			errs = append(errs, err)
		}
	} else if unknownEnabled {
		cfgBytes, err := os.ReadFile(o.config.ConfigPath)
		if err != nil {
			return fmt.Errorf("error reading Prow config for validation: %w", err)
		}
		if err := validateUnknownFields(&config.Config{}, cfgBytes, o.config.ConfigPath); err != nil {
			errs = append(errs, err)
		}
	}
	if pcfg != nil && (unknownEnabled || unknownAllEnabled) {
		pcfgBytes, err := os.ReadFile(o.pluginsConfig.PluginConfigPath)
		if err != nil {
			return fmt.Errorf("error reading Prow plugin config for validation: %w", err)
		}
		if err := validateUnknownFields(&plugins.Configuration{}, pcfgBytes, o.pluginsConfig.PluginConfigPath); err != nil {
			errs = append(errs, err)
		}
	}
	if o.warningEnabled(tideStrictBranchWarning) {
		if err := validateStrictBranches(cfg.ProwConfig); err != nil {
			errs = append(errs, err)
		}
	}
	if o.warningEnabled(tideContextPolicy) {
		if err := validateTideContextPolicy(cfg); err != nil {
			errs = append(errs, err)
		}
	}
	if o.warningEnabled(validateClusterFieldWarning) {
		opener, err := io.NewOpener(context.Background(), o.storage.GCSCredentialsFile, o.storage.S3CredentialsFile)
		if err != nil {
			logrus.WithError(err).Fatal("Error creating opener")
		}
		if err := validateCluster(cfg, opener); err != nil {
			errs = append(errs, err)
		}
	}

	if o.warningEnabled(validateSupplementalProwConfigOrgRepoHirarchy) {
		if err := validateAdditionalProwConfigIsInOrgRepoDirectoryStructure(os.DirFS("./"), o.config.SupplementalProwConfigDirs.Strings(), o.pluginsConfig.SupplementalPluginsConfigDirs.Strings(), o.config.SupplementalProwConfigsFileNameSuffix, o.pluginsConfig.SupplementalPluginsConfigsFileNameSuffix); err != nil {
			errs = append(errs, err)
		}
	}

	if o.warningEnabled(validateUnmanagedBranchConfigHasNoSubconfig) {
		if err := validateUnmanagedBranchprotectionConfigDoesntHaveSubconfig(cfg.BranchProtection); err != nil {
			errs = append(errs, err)
		}
	}

	if o.warningEnabled(validateGitHubAppInstallationWarning) {
		githubClient, err := o.github.GitHubClient(false)
		if err != nil {
			return fmt.Errorf("error loading GitHub client: %w", err)
		}

		if err := validateGitHubAppIsInstalled(githubClient, cfg.AllRepos); err != nil {
			errs = append(errs, err)
		}
	}

	if pcfg != nil && o.warningEnabled(validateLabelWarning) {
		if err := verifyLabelPlugin(pcfg.Label); err != nil {
			errs = append(errs, err)
		}
	}

	if o.warningEnabled(requiredJobAnnotationsWarning) {
		if err := validateRequiredJobAnnotations(o.requiredJobAnnotations.Strings(), cfg.JobConfig); err != nil {
			errs = append(errs, err)
		}
	}

	// validate rerun commands match presubmit job triggering regex
	for _, presubmits := range cfg.JobConfig.PresubmitsStatic {
		for _, p := range presubmits {
			if !cfg.Gerrit.IsAllowedPresubmitTrigger(p.RerunCommand) {
				errs = append(errs, fmt.Errorf("rerun command %s in job %s does not conform to test command requirements,"+
					"please make sure the trigger regex is a subset of %s and the rerun command matches the trigger regex",
					p.RerunCommand, p.Name, cfg.Gerrit.AllowedPresubmitTriggerReRawString))
			}
		}
	}

	return utilerrors.NewAggregate(errs)
}
func policyIsStrict(p config.Policy) bool {
	if p.Protect == nil || !*p.Protect {
		return false
	}
	if p.RequiredStatusChecks == nil || p.RequiredStatusChecks.Strict == nil {
		return false
	}
	return *p.RequiredStatusChecks.Strict
}

func strictBranchesConfig(c config.ProwConfig) (*orgRepoConfig, error) {
	strictOrgExceptions := make(map[string]sets.Set[string])
	strictRepos := sets.New[string]()
	for orgName := range c.BranchProtection.Orgs {
		org := c.BranchProtection.GetOrg(orgName)
		// First find explicitly configured repos and partition based on strictness.
		// If any branch in a repo is strict we assume that the whole repo is to
		// simplify this validation.
		strictExplicitRepos, nonStrictExplicitRepos := sets.New[string](), sets.New[string]()
		for repoName := range org.Repos {
			repo := org.GetRepo(repoName)
			strict := policyIsStrict(repo.Policy)
			if !strict {
				for branchName := range repo.Branches {
					branch, err := repo.GetBranch(branchName)
					if err != nil {
						return nil, fmt.Errorf("error for repo=%s/%s and branch=%s: %w",
							orgName, repoName, branchName, err)
					}
					if policyIsStrict(branch.Policy) {
						strict = true
						break
					}
				}
			}
			fullRepoName := fmt.Sprintf("%s/%s", orgName, repoName)
			if strict {
				strictExplicitRepos.Insert(fullRepoName)
			} else {
				nonStrictExplicitRepos.Insert(fullRepoName)
			}
		}
		// Done partitioning the repos.

		if policyIsStrict(org.Policy) {
			// This org is strict, record with repo exceptions ("denylist")
			strictOrgExceptions[orgName] = nonStrictExplicitRepos
		} else {
			// The org is not strict, record member repos that are allowed
			strictRepos.Insert(strictExplicitRepos.UnsortedList()...)
		}
	}
	return newOrgRepoConfig(strictOrgExceptions, strictRepos), nil
}

func validateStrictBranches(c config.ProwConfig) error {
	const explanation = "See #5: https://github.com/kubernetes/test-infra/blob/master/prow/cmd/tide/maintainers.md#best-practices Also note that this validation is imperfect, see the check-config code for details"
	if len(c.Tide.Queries) == 0 {
		// Short circuit here so that we can allow global level branchprotector
		// 'strict: true' if Tide is not enabled.
		// Ignoring the case where Tide is enabled only on orgs/repos specifically
		// exempted from the global setting simplifies validation immensely.
		return nil
	}
	if policyIsStrict(c.BranchProtection.Policy) {
		return fmt.Errorf("strict branchprotection context requirements cannot be globally enabled when Tide is configured for use. %s", explanation)
	}
	// The two assumptions below are not necessarily true, but they hold for all
	// known instances and make this validation much simpler.

	// Assumes if any branch is managed by Tide, the whole repo is.
	overallTideConfig := newOrgRepoConfig(c.Tide.Queries.OrgExceptionsAndRepos())
	// Assumes if any branch is strict the repo is strict.
	strictBranchConfig, err := strictBranchesConfig(c)
	if err != nil {
		return err
	}

	conflicts := overallTideConfig.intersection(strictBranchConfig).items()
	if len(conflicts) == 0 {
		return nil
	}
	return fmt.Errorf(
		"the following enable strict branchprotection context requirements even though Tide handles merges: [%s]. %s",
		strings.Join(conflicts, "; "),
		explanation,
	)
}

func validateURLs(c config.ProwConfig) error {
	var validationErrs []error

	if _, err := url.Parse(c.StatusErrorLink); err != nil {
		validationErrs = append(validationErrs, fmt.Errorf("status_error_link is not a valid url: %s", c.StatusErrorLink))
	}

	return utilerrors.NewAggregate(validationErrs)
}

func validateUnknownFields(cfg interface{}, cfgBytes []byte, filePath string) error {
	err := yaml.Unmarshal(cfgBytes, &cfg, yaml.DisallowUnknownFields)
	if err != nil {
		return fmt.Errorf("unknown fields or bad config in %s: %w", filePath, err)
	}
	return nil
}

func validateJobRequirements(c config.JobConfig) error {
	var validationErrs []error
	for repo, jobs := range c.PresubmitsStatic {
		for _, job := range jobs {
			validationErrs = append(validationErrs, validatePresubmitJob(repo, job))
		}
	}
	for repo, jobs := range c.PostsubmitsStatic {
		for _, job := range jobs {
			validationErrs = append(validationErrs, validatePostsubmitJob(repo, job))
		}
	}
	for _, job := range c.Periodics {
		validationErrs = append(validationErrs, validatePeriodicJob(job))
	}

	return utilerrors.NewAggregate(validationErrs)
}

func validatePresubmitJob(repo string, job config.Presubmit) error {
	var validationErrs []error
	// Prow labels k8s resources with job names. Labels are capped at 63 chars.
	if job.Agent == string(v1.KubernetesAgent) && len(job.Name) > validation.LabelValueMaxLength {
		validationErrs = append(validationErrs, fmt.Errorf("name of Presubmit job %q (for repo %q) too long (should be at most 63 characters)", job.Name, repo))
	}
	return utilerrors.NewAggregate(validationErrs)
}

func validatePostsubmitJob(repo string, job config.Postsubmit) error {
	var validationErrs []error
	// Prow labels k8s resources with job names. Labels are capped at 63 chars.
	if job.Agent == string(v1.KubernetesAgent) && len(job.Name) > validation.LabelValueMaxLength {
		validationErrs = append(validationErrs, fmt.Errorf("name of Postsubmit job %q (for repo %q) too long (should be at most 63 characters)", job.Name, repo))
	}
	return utilerrors.NewAggregate(validationErrs)
}

func validateJobExtraRefs(cfg config.JobConfig) error {
	var validationErrs []error
	for repo, presubmits := range cfg.PresubmitsStatic {
		for _, presubmit := range presubmits {
			if err := config.ValidateRefs(repo, presubmit.JobBase); err != nil {
				validationErrs = append(validationErrs, err)
			}
		}
	}
	return utilerrors.NewAggregate(validationErrs)
}

func validatePeriodicDefaultCloneConfig(cfg config.JobConfig) error {
	var validationErrs []error
	for _, job := range cfg.Periodics {
		// Top level clone configs don't make sense for periodics jobs.
		if job.CloneDepth != 0 || job.CloneURI != "" || job.PathAlias != "" {
			validationErrs = append(validationErrs, fmt.Errorf("periodic jobs might clone 0, 1, or more repos, top level `clone_depth`, `clone_uri`, and `path_alias` don't have any effect. Name: %q", job.Name))
		}
	}
	return utilerrors.NewAggregate(validationErrs)
}

func validatePeriodicJob(job config.Periodic) error {
	var validationErrs []error
	// Prow labels k8s resources with job names. Labels are capped at 63 chars.
	if job.Agent == string(v1.KubernetesAgent) && len(job.Name) > validation.LabelValueMaxLength {
		validationErrs = append(validationErrs, fmt.Errorf("name of Periodic job %q too long (should be at most 63 characters)", job.Name))
	}
	return utilerrors.NewAggregate(validationErrs)
}

func validateTideRequirements(cfg *config.Config, pcfg *plugins.Configuration, includeForbidden bool) error {
	type matcher struct {
		// matches determines if the tide query appropriately honors the
		// label in question -- whether by requiring it or forbidding it
		matches func(label string, query config.TideQuery) bool
		// verb is used in forming error messages
		verb string
	}
	requires := matcher{
		matches: func(label string, query config.TideQuery) bool {
			return sets.New[string](query.Labels...).Has(label)
		},
		verb: "require",
	}
	forbids := matcher{
		matches: func(label string, query config.TideQuery) bool {
			return sets.New[string](query.MissingLabels...).Has(label)
		},
		verb: "forbid",
	}

	type plugin struct {
		// name and label identify the relationship we are validating
		name, label string
		// external indicates plugin is external or not
		external bool
		// matcher determines if the tide query appropriately honors the
		// label in question -- whether by requiring it or forbidding it
		matcher matcher
		// config holds the orgs and repos for which tide does honor the
		// label; this container is populated conditionally from queries
		// using the matcher
		config *orgRepoConfig
	}
	// configs list relationships between tide config
	// and plugin enablement that we want to validate
	configs := []plugin{
		{name: lgtm.PluginName, label: labels.LGTM, matcher: requires},
		{name: approve.PluginName, label: labels.Approved, matcher: requires},
	}
	if includeForbidden {
		configs = append(configs,
			plugin{name: hold.PluginName, label: labels.Hold, matcher: forbids},
			plugin{name: wip.PluginName, label: labels.WorkInProgress, matcher: forbids},
			plugin{name: bugzilla.PluginName, label: labels.InvalidBug, matcher: forbids},
			plugin{name: verifyowners.PluginName, label: labels.InvalidOwners, matcher: forbids},
			plugin{name: releasenote.PluginName, label: labels.ReleaseNoteLabelNeeded, matcher: forbids},
			plugin{name: cherrypickunapproved.PluginName, label: labels.CpUnapproved, matcher: forbids},
			plugin{name: blockade.PluginName, label: labels.BlockedPaths, matcher: forbids},
			plugin{name: needsrebase.PluginName, label: labels.NeedsRebase, external: true, matcher: forbids},
		)
	}

	for i := range configs {
		// For each plugin determine the subset of tide queries that match and then
		// the orgs and repos that the subset matches.
		var matchingQueries config.TideQueries
		for _, query := range cfg.Tide.Queries {
			if configs[i].matcher.matches(configs[i].label, query) {
				matchingQueries = append(matchingQueries, query)
			}
		}
		configs[i].config = newOrgRepoConfig(matchingQueries.OrgExceptionsAndRepos())
	}

	overallTideConfig := newOrgRepoConfig(cfg.Tide.Queries.OrgExceptionsAndRepos())

	// Now actually execute the checks we just configured.
	var validationErrs []error
	for _, pluginConfig := range configs {
		err := ensureValidConfiguration(
			pluginConfig.name,
			pluginConfig.label,
			pluginConfig.matcher.verb,
			pluginConfig.config,
			overallTideConfig,
			enabledOrgReposForPlugin(pcfg, pluginConfig.name, pluginConfig.external),
		)
		validationErrs = append(validationErrs, err)
	}

	return utilerrors.NewAggregate(validationErrs)
}

func newOrgRepoConfig(orgExceptions map[string]sets.Set[string], repos sets.Set[string]) *orgRepoConfig {
	return &orgRepoConfig{
		orgExceptions: orgExceptions,
		repos:         repos,
	}
}

// orgRepoConfig describes a set of repositories with an explicit
// allowlist and a mapping of denied repos for owning orgs
type orgRepoConfig struct {
	// orgExceptions holds explicit denylists of repos for owning orgs
	orgExceptions map[string]sets.Set[string]
	// repos is an allowed list of repos
	repos sets.Set[string]
}

func (c *orgRepoConfig) items() []string {
	items := make([]string, 0, len(c.orgExceptions)+len(c.repos))
	for org, excepts := range c.orgExceptions {
		item := fmt.Sprintf("org: %s", org)
		if excepts.Len() > 0 {
			item = fmt.Sprintf("%s without repo(s) %s", item, strings.Join(sets.List(excepts), ", "))
			for _, repo := range sets.List(excepts) {
				item = fmt.Sprintf("%s '%s'", item, repo)
			}
		}
		items = append(items, item)
	}
	for _, repo := range sets.List(c.repos) {
		items = append(items, fmt.Sprintf("repo: %s", repo))
	}
	return items
}

// difference returns a new orgRepoConfig that represents the set difference of
// the repos specified by the receiver and the parameter orgRepoConfigs.
func (c *orgRepoConfig) difference(c2 *orgRepoConfig) *orgRepoConfig {
	res := &orgRepoConfig{
		orgExceptions: make(map[string]sets.Set[string]),
		repos:         sets.New[string]().Union(c.repos),
	}
	for org, excepts1 := range c.orgExceptions {
		if excepts2, ok := c2.orgExceptions[org]; ok {
			res.repos.Insert(excepts2.Difference(excepts1).UnsortedList()...)
		} else {
			excepts := sets.New[string]().Union(excepts1)
			// Add any applicable repos in repos2 to excepts
			for _, repo := range c2.repos.UnsortedList() {
				if parts := strings.SplitN(repo, "/", 2); len(parts) == 2 && parts[0] == org {
					excepts.Insert(repo)
				}
			}
			res.orgExceptions[org] = excepts
		}
	}

	res.repos = res.repos.Difference(c2.repos)

	for _, repo := range res.repos.UnsortedList() {
		if parts := strings.SplitN(repo, "/", 2); len(parts) == 2 {
			if excepts2, ok := c2.orgExceptions[parts[0]]; ok && !excepts2.Has(repo) {
				res.repos.Delete(repo)
			}
		}
	}
	return res
}

// intersection returns a new orgRepoConfig that represents the set intersection
// of the repos specified by the receiver and the parameter orgRepoConfigs.
func (c *orgRepoConfig) intersection(c2 *orgRepoConfig) *orgRepoConfig {
	res := &orgRepoConfig{
		orgExceptions: make(map[string]sets.Set[string]),
		repos:         sets.New[string](),
	}
	for org, excepts1 := range c.orgExceptions {
		// Include common orgs, but union exceptions.
		if excepts2, ok := c2.orgExceptions[org]; ok {
			res.orgExceptions[org] = excepts1.Union(excepts2)
		} else {
			// Include right side repos that match left side org.
			for _, repo := range c2.repos.UnsortedList() {
				if parts := strings.SplitN(repo, "/", 2); len(parts) == 2 && parts[0] == org && !excepts1.Has(repo) {
					res.repos.Insert(repo)
				}
			}
		}
	}
	for _, repo := range c.repos.UnsortedList() {
		if c2.repos.Has(repo) {
			res.repos.Insert(repo)
		} else if parts := strings.SplitN(repo, "/", 2); len(parts) == 2 {
			// Include left side repos that match right side org.
			if excepts2, ok := c2.orgExceptions[parts[0]]; ok && !excepts2.Has(repo) {
				res.repos.Insert(repo)
			}
		}
	}
	return res
}

// union returns a new orgRepoConfig that represents the set union of the
// repos specified by the receiver and the parameter orgRepoConfigs
func (c *orgRepoConfig) union(c2 *orgRepoConfig) *orgRepoConfig {
	res := &orgRepoConfig{
		orgExceptions: make(map[string]sets.Set[string]),
		repos:         sets.New[string](),
	}

	for org, excepts1 := range c.orgExceptions {
		// keep only items in both denylists that are not in the
		// explicit repo allowlist for the other configuration;
		// we know from how the orgRepoConfigs are constructed that
		// a org denylist won't intersect it's own repo allowlist
		pruned := excepts1.Difference(c2.repos)
		if excepts2, ok := c2.orgExceptions[org]; ok {
			res.orgExceptions[org] = pruned.Intersection(excepts2.Difference(c.repos))
		} else {
			res.orgExceptions[org] = pruned
		}
	}

	for org, excepts2 := range c2.orgExceptions {
		// update any denylists not previously updated
		if _, exists := res.orgExceptions[org]; !exists {
			res.orgExceptions[org] = excepts2.Difference(c.repos)
		}
	}

	// we need to prune out repos in the allowed lists which are
	// covered by an org already; we know from above that no
	// org denylist in the result will contain a repo allowlist
	for _, repo := range c.repos.Union(c2.repos).UnsortedList() {
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) != 2 {
			logrus.Warnf("org/repo %q is formatted incorrectly", repo)
			continue
		}
		if _, exists := res.orgExceptions[parts[0]]; !exists {
			res.repos.Insert(repo)
		}
	}
	return res
}

func enabledOrgReposForPlugin(c *plugins.Configuration, plugin string, external bool) *orgRepoConfig {
	var (
		orgs  []string
		repos []string
	)
	var orgMap map[string]sets.Set[string]
	if external {
		orgs, repos = c.EnabledReposForExternalPlugin(plugin)
		orgMap = make(map[string]sets.Set[string], len(orgs))
		for _, org := range orgs {
			orgMap[org] = nil
		}
	} else {
		_, repos, orgMap = c.EnabledReposForPlugin(plugin)
	}
	return newOrgRepoConfig(orgMap, sets.New[string](repos...))
}

// ensureValidConfiguration enforces rules about tide and plugin config.
// In this context, a subset is the set of repos or orgs for which a specific
// plugin is either enabled (for plugins) or required for merge (for tide). The
// tide superset is every org or repo that has any configuration at all in tide.
// Specifically:
//   - every item in the tide subset must also be in the plugins subset
//   - every item in the plugins subset that is in the tide superset must also be in the tide subset
//
// For example:
//   - if org/repo is configured in tide to require lgtm, it must have the lgtm plugin enabled
//   - if org/repo is configured in tide, the tide configuration must require the same set of
//     plugins as are configured. If the repository has LGTM and approve enabled, the tide query
//     must require both labels
func ensureValidConfiguration(plugin, label, verb string, tideSubSet, tideSuperSet, pluginsSubSet *orgRepoConfig) error {
	notEnabled := tideSubSet.difference(pluginsSubSet).items()
	notRequired := pluginsSubSet.intersection(tideSuperSet).difference(tideSubSet).items()

	var configErrors []error
	if len(notEnabled) > 0 {
		configErrors = append(configErrors, fmt.Errorf("the following orgs or repos %s the %s label for merging but do not enable the %s plugin: %v", verb, label, plugin, notEnabled))
	}
	if len(notRequired) > 0 {
		configErrors = append(configErrors, fmt.Errorf("the following orgs or repos enable the %s plugin but do not %s the %s label for merging: %v", plugin, verb, label, notRequired))
	}

	return utilerrors.NewAggregate(configErrors)
}

func validateDecoratedJobs(cfg *config.Config) error {
	var nonDecoratedJobs []string
	for _, presubmit := range cfg.AllStaticPresubmits([]string{}) {
		if presubmit.Agent == string(v1.KubernetesAgent) && !*presubmit.JobBase.UtilityConfig.Decorate {
			nonDecoratedJobs = append(nonDecoratedJobs, presubmit.Name)
		}
	}

	for _, postsubmit := range cfg.AllStaticPostsubmits([]string{}) {
		if postsubmit.Agent == string(v1.KubernetesAgent) && !*postsubmit.JobBase.UtilityConfig.Decorate {
			nonDecoratedJobs = append(nonDecoratedJobs, postsubmit.Name)
		}
	}

	for _, periodic := range cfg.AllPeriodics() {
		if periodic.Agent == string(v1.KubernetesAgent) && !*periodic.JobBase.UtilityConfig.Decorate {
			nonDecoratedJobs = append(nonDecoratedJobs, periodic.Name)
		}
	}

	if len(nonDecoratedJobs) > 0 {
		return fmt.Errorf("the following jobs use the kubernetes provider but do not use the pod utilities: %v", nonDecoratedJobs)
	}
	return nil
}

func validateDecorationConfig(cfg *config.Config) error {
	var configErrors []error
	for _, presubmit := range cfg.AllStaticPresubmits([]string{}) {
		if presubmit.Agent == string(v1.KubernetesAgent) && presubmit.Decorate != nil && *presubmit.Decorate && presubmit.DecorationConfig != nil {
			if err := presubmit.DecorationConfig.Validate(); err != nil {
				configErrors = append(configErrors, err)
			}
		}
	}

	for _, postsubmit := range cfg.AllStaticPostsubmits([]string{}) {
		if postsubmit.Agent == string(v1.KubernetesAgent) && postsubmit.Decorate != nil && *postsubmit.Decorate && postsubmit.DecorationConfig != nil {
			if err := postsubmit.DecorationConfig.Validate(); err != nil {
				configErrors = append(configErrors, err)
			}
		}
	}

	for _, periodic := range cfg.AllPeriodics() {
		if periodic.Agent == string(v1.KubernetesAgent) && periodic.Decorate != nil && *periodic.Decorate && periodic.DecorationConfig != nil {
			if err := periodic.DecorationConfig.Validate(); err != nil {
				configErrors = append(configErrors, err)
			}
		}
	}
	return utilerrors.NewAggregate(configErrors)
}

func validateNeedsOkToTestLabel(cfg *config.Config) error {
	var queryErrors []error
	for i, query := range cfg.Tide.Queries {
		for _, label := range query.Labels {
			if label == lgtm.LGTMLabel {
				for _, label := range query.MissingLabels {
					if label == labels.NeedsOkToTest {
						queryErrors = append(queryErrors, fmt.Errorf(
							"the tide query at position %d"+
								"forbids the %q label and requires the %q label, "+
								"which is not recommended; "+
								"see https://github.com/kubernetes/test-infra/blob/master/prow/cmd/tide/maintainers.md#best-practices "+
								"for more information",
							i, labels.NeedsOkToTest, lgtm.LGTMLabel),
						)
					}
				}
			}
		}
	}
	return utilerrors.NewAggregate(queryErrors)
}

func validateManagedWebhooks(cfg *config.Config) error {
	mw := cfg.ManagedWebhooks
	var errs []error
	orgs := sets.Set[string]{}
	for repo := range mw.OrgRepoConfig {
		if !strings.Contains(repo, "/") {
			org := repo
			orgs.Insert(org)
		}
	}
	for repo := range mw.OrgRepoConfig {
		if strings.Contains(repo, "/") {
			org := strings.SplitN(repo, "/", 2)[0]
			if orgs.Has(org) {
				errs = append(errs, fmt.Errorf(
					"org-level and repo-level webhooks are configured together for %q, "+
						"which is not allowed as there will be duplicated webhook events", repo))
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}

func pluginsWithOwnersFile() string {
	return strings.Join([]string{approve.PluginName, blunderbuss.PluginName, ownerslabel.PluginName}, ", ")
}

func orgReposUsingOwnersFile(cfg *plugins.Configuration) *orgRepoConfig {
	// we do not know the set of repos that use OWNERS, but we
	// can get a reasonable proxy for this by looking at where
	// the `approve', `blunderbuss' and `owners-label' plugins
	// are enabled
	approveConfig := enabledOrgReposForPlugin(cfg, approve.PluginName, false)
	blunderbussConfig := enabledOrgReposForPlugin(cfg, blunderbuss.PluginName, false)
	ownersLabelConfig := enabledOrgReposForPlugin(cfg, ownerslabel.PluginName, false)
	return approveConfig.union(blunderbussConfig).union(ownersLabelConfig)
}

type FileInRepoExistsChecker interface {
	GetRepos(org string, isUser bool) ([]github.Repo, error)
	GetFile(org, repo, filepath, commit string) ([]byte, error)
}

func verifyOwnersPresence(cfg *plugins.Configuration, rc FileInRepoExistsChecker) error {
	ownersConfig := orgReposUsingOwnersFile(cfg)

	var missing []string
	for org, excluded := range ownersConfig.orgExceptions {
		repos, err := rc.GetRepos(org, false)
		if err != nil {
			return err
		}

		for _, repo := range repos {
			if excluded.Has(repo.FullName) || repo.Archived {
				continue
			}
			if _, err := rc.GetFile(repo.Owner.Login, repo.Name, "OWNERS", ""); err != nil {
				if _, nf := err.(*github.FileNotFound); nf {
					missing = append(missing, repo.FullName)
				} else {
					return fmt.Errorf("got error: %w", err)
				}
			}
		}
	}

	for repo := range ownersConfig.repos {
		items := strings.Split(repo, "/")
		if len(items) != 2 {
			return fmt.Errorf("bad repository '%s', expected org/repo format", repo)
		}
		if _, err := rc.GetFile(items[0], items[1], "OWNERS", ""); err != nil {
			if _, nf := err.(*github.FileNotFound); nf {
				missing = append(missing, repo)
			} else {
				return fmt.Errorf("got error: %w", err)
			}
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("the following orgs or repos enable at least one"+
			" plugin that uses OWNERS files (%s), but its master branch does not contain"+
			" a root level OWNERS file: %v", pluginsWithOwnersFile(), missing)
	}
	return nil
}

func verifyOwnersPlugin(cfg *plugins.Configuration) error {
	ownersConfig := orgReposUsingOwnersFile(cfg)
	validateOwnersConfig := enabledOrgReposForPlugin(cfg, verifyowners.PluginName, false)

	invalid := ownersConfig.difference(validateOwnersConfig).items()
	if len(invalid) > 0 {
		return fmt.Errorf("the following orgs or repos "+
			"enable at least one plugin that uses OWNERS files (%s) "+
			"but do not enable the %s plugin to ensure validity of OWNERS files: %v",
			pluginsWithOwnersFile(), verifyowners.PluginName, invalid,
		)
	}
	return nil
}

func verifyLabelPlugin(label plugins.Label) error {
	var orgReposWithEmptyLabelConfig []string
	var errs []error
	restrictedAndAdditionalLabels := make(map[string][]string)
	for orgRepo, restrictedLabels := range label.RestrictedLabels {
		for _, restrictedLabel := range restrictedLabels {
			if label.IsRestrictedLabelInAdditionalLables(restrictedLabel.Label) {
				restrictedAndAdditionalLabels[restrictedLabel.Label] = append(restrictedAndAdditionalLabels[restrictedLabel.Label], orgRepo)
			}
			if restrictedLabel.Label == "" {
				orgReposWithEmptyLabelConfig = append(orgReposWithEmptyLabelConfig, orgRepo)
			}
		}
	}

	for label, repos := range restrictedAndAdditionalLabels {
		sort.Strings(repos)
		errs = append(errs,
			fmt.Errorf("the following orgs or repos have configuration of label plugin using the restricted label %s which is also configured as an additional label: %s", label, strings.Join(repos, ", ")))
	}

	if len(orgReposWithEmptyLabelConfig) > 0 {
		sort.Strings(orgReposWithEmptyLabelConfig)
		errs = append(errs, fmt.Errorf("the following orgs or repos have configuration of %s plugin using the empty string as label name in restricted labels: %s",
			labelplugin.PluginName, strings.Join(orgReposWithEmptyLabelConfig, ", "),
		))
	}
	return utilerrors.NewAggregate(errs)
}

func validateTriggers(cfg *config.Config, pcfg *plugins.Configuration) error {
	configuredRepos := sets.New[string]()
	for orgRepo := range cfg.JobConfig.PresubmitsStatic {
		configuredRepos.Insert(orgRepo)
	}
	for orgRepo := range cfg.JobConfig.PostsubmitsStatic {
		configuredRepos.Insert(orgRepo)
	}

	configured := newOrgRepoConfig(map[string]sets.Set[string]{}, configuredRepos)
	enabled := enabledOrgReposForPlugin(pcfg, trigger.PluginName, false)

	if missing := configured.difference(enabled).items(); len(missing) > 0 {
		return fmt.Errorf("the following repos have jobs configured but do not have the %s plugin enabled: %s", trigger.PluginName, strings.Join(missing, ", "))
	}
	return nil
}

func validateInRepoConfig(cfg *config.Config, filepath, repoIdentifier string, strict bool) error {
	var dir string
	var err error
	// Unfortunately we must continue to support the filepath arg for existing uses.
	if filepath != "" {
		dir = path.Dir(filepath)
	} else {
		if dir, err = os.Getwd(); err != nil {
			return fmt.Errorf("failed to get current working directory")
		}
	}
	prowYAML, err := config.ReadProwYAML(logrus.WithField("repo", repoIdentifier), dir, strict)
	if err != nil {
		return fmt.Errorf("failed to read Prow YAML: %w", err)
	}

	if err := config.DefaultAndValidateProwYAML(cfg, prowYAML, repoIdentifier); err != nil {
		return fmt.Errorf("failed to validate Prow YAML: %w", err)
	}

	var errs []error
	for _, pre := range prowYAML.Presubmits {
		if !cfg.Gerrit.IsAllowedPresubmitTrigger(pre.RerunCommand) {
			errs = append(errs, fmt.Errorf("rerun command %s in job %s does not conform to test command requirements,"+
				"please make sure the trigger regex is a subset of %s and the rerun command matches the trigger regex",
				pre.RerunCommand, pre.Name, cfg.Gerrit.AllowedPresubmitTriggerReRawString))
		}
	}
	if len(errs) > 0 {
		return utilerrors.NewAggregate(errs)
	}

	return nil
}

func validateTideContextPolicy(cfg *config.Config) error {
	// We can not know all possible branches without asking GitHub, so instead we verify
	// all branches that are explicitly configured on any job. This will hopefully catch
	// most cases.
	allKnownOrgRepoBranches := map[string]sets.Set[string]{}
	for orgRepo, jobs := range cfg.PresubmitsStatic {
		if _, ok := allKnownOrgRepoBranches[orgRepo]; !ok {
			allKnownOrgRepoBranches[orgRepo] = sets.Set[string]{}
		}

		for _, job := range jobs {
			allKnownOrgRepoBranches[orgRepo].Insert(job.Branches...)
		}
	}

	// We have to disableInRepoConfig for this check, else we will
	// attempt to clone the repo if its enabled
	originalInRepoConfig := cfg.InRepoConfig
	cfg.InRepoConfig = config.InRepoConfig{}
	defer func() { cfg.InRepoConfig = originalInRepoConfig }()

	var errs []error
	for orgRepo, branches := range allKnownOrgRepoBranches {
		split := strings.Split(orgRepo, "/")
		if n := len(split); n != 2 {
			// May happen for gerrit
			continue
		}
		org, repo := split[0], split[1]

		if branches.Len() == 0 {
			// Make sure we always test at least one branch per repo
			// to catch cases where ppl only have jobs with empty branch
			// configs.
			branches.Insert("master")
		}
		for _, branch := range sets.List(branches) {
			if _, err := cfg.GetTideContextPolicy(nil, org, repo, branch, nil, ""); err != nil {
				errs = append(errs, fmt.Errorf("context policy for %s branch in %s/%s is invalid: %w", branch, org, repo, err))
			}
		}
	}

	return utilerrors.NewAggregate(errs)
}

var agentsNotSupportingCluster = sets.New[string]("jenkins")

func validateJobCluster(job config.JobBase, statuses map[string]plank.ClusterStatus) error {
	if job.Cluster != "" && job.Cluster != kube.DefaultClusterAlias && agentsNotSupportingCluster.Has(job.Agent) {
		return fmt.Errorf("%s: cannot set cluster field if agent is %s", job.Name, job.Agent)
	}
	if statuses != nil {
		status, ok := statuses[job.Cluster]
		if !ok {
			return fmt.Errorf("job configuration for %q specifies unknown 'cluster' value %q", job.Name, job.Cluster)
		}
		if status != plank.ClusterStatusReachable {
			logrus.Warnf("Job configuration for %q specifies cluster %q which cannot be reached from Plank. Status: %q", job.Name, job.Cluster, status)
		}
	}
	return nil
}

func validateCluster(cfg *config.Config, opener io.Opener) error {
	var statuses map[string]plank.ClusterStatus
	if location := cfg.Plank.BuildClusterStatusFile; location != "" {
		reader, err := opener.Reader(context.Background(), location)
		if err != nil {
			if !io.IsNotExist(err) {
				return fmt.Errorf("error opening build cluster status file for reading: %w", err)
			}
			logrus.Warnf("Build cluster status file location was specified, but could not be found: %v. This is expected when the location is first configured, before plank creates the file.", err)
		} else {
			defer reader.Close()
			b, err := stdio.ReadAll(reader)
			if err != nil {
				return fmt.Errorf("error reading build cluster status file: %w", err)
			}
			statuses = map[string]plank.ClusterStatus{}
			if err := json.Unmarshal(b, &statuses); err != nil {
				return fmt.Errorf("error unmarshaling build cluster status file: %w", err)
			}
		}
	}
	var errs []error
	for orgRepo, jobs := range cfg.PresubmitsStatic {
		for _, job := range jobs {
			if err := validateJobCluster(job.JobBase, statuses); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", orgRepo, err))
			}
		}
	}
	for _, job := range cfg.Periodics {
		if err := validateJobCluster(job.JobBase, statuses); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", "invalid periodic job", err))
		}

	}
	for orgRepo, jobs := range cfg.PostsubmitsStatic {
		for _, job := range jobs {
			if err := validateJobCluster(job.JobBase, statuses); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", orgRepo, err))
			}
		}
	}
	return utilerrors.NewAggregate(errs)
}

func validateAdditionalProwConfigIsInOrgRepoDirectoryStructure(filesystem fs.FS, supplementalProwConfigDirs, supplementalPluginsConfigDirs []string, supplementalProwConfigsFileNameSuffix, supplementalPluginsConfigFileNameSuffix string) error {
	var errs []error

	for _, supplementalProwConfigDir := range supplementalPluginsConfigDirs {
		if err := validateAdditionalConfigIsInOrgRepoDirectoryStructure(supplementalProwConfigDir, filesystem, func() hierarchicalConfig { return &config.Config{} }, supplementalProwConfigsFileNameSuffix); err != nil {
			errs = append(errs, err)
		}
	}
	for _, supplementalPluginsConfigDir := range supplementalPluginsConfigDirs {
		if err := validateAdditionalConfigIsInOrgRepoDirectoryStructure(supplementalPluginsConfigDir, filesystem, func() hierarchicalConfig { return &plugins.Configuration{} }, supplementalPluginsConfigFileNameSuffix); err != nil {
			errs = append(errs, err)
		}
	}

	return utilerrors.NewAggregate(errs)
}

func validateAdditionalConfigIsInOrgRepoDirectoryStructure(root string, filesystem fs.FS, target func() hierarchicalConfig, filesuffix string) error {
	var errs []error
	errs = append(errs, fs.WalkDir(filesystem, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, fmt.Errorf("error when walking: %w", err))
			return nil
		}
		// Kubernetes configmap mounts create symlinks for the configmap keys that point to files prefixed with '..'.
		// This allows it to do  atomic changes by changing the symlink to a new target when the configmap content changes.
		// This means that we should ignore the '..'-prefixed files, otherwise we might end up reading a half-written file and will
		// get duplicate data.
		if strings.HasPrefix(d.Name(), "..") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		fs.ReadFile(filesystem, path)

		if d.IsDir() || !strings.HasSuffix(path, filesuffix) {
			return nil
		}

		pathWithoutRoot := strings.TrimPrefix(path, root)
		pathWithoutRoot = strings.TrimPrefix(pathWithoutRoot, "/")

		pathElements := strings.Split(pathWithoutRoot, "/")
		nestingDepth := len(pathElements) - 1

		var isOrgConfig, isRepoConfig bool
		switch nestingDepth {
		case 0:
			// Global config, might contain anything or not even be a Prow config
			return nil
		case 1:
			isOrgConfig = true
		case 2:
			isRepoConfig = true
		default:
			errs = append(errs, fmt.Errorf("config %s is at an invalid location. All configs must be below %s. If they are org-specific, they must be in a folder named like the org. If they are repo-specific, they must be in a folder named like the repo below a folder named like the org.", path, root))
			return nil
		}

		cfg := target()
		isGlobal, targetedOrgs, targetedRepos, err := getSupplementalConfigScope(path, filesystem, cfg)
		if err != nil {
			errs = append(errs, err)
			return nil
		}

		if isOrgConfig {
			expectedTargetOrg := pathElements[0]
			if !isGlobal && len(targetedOrgs) == 1 && targetedOrgs.Has(expectedTargetOrg) && len(targetedRepos) == 0 {
				return nil
			}
			errMsg := fmt.Sprintf("config %s is invalid: Must contain only config for org %s, but", path, expectedTargetOrg)
			var needsAnd bool
			if isGlobal {
				errMsg += " contains global config"
				needsAnd = true
			}
			for _, org := range sets.List(targetedOrgs.Delete(expectedTargetOrg)) {
				errMsg += prefixWithAndIfNeeded(fmt.Sprintf(" contains config for org %s", org), needsAnd)
				needsAnd = true
			}
			for _, repo := range sets.List(targetedRepos) {
				errMsg += prefixWithAndIfNeeded(fmt.Sprintf(" contains config for repo %s", repo), needsAnd)
				needsAnd = true
			}
			errs = append(errs, errors.New(errMsg))
			return nil
		}

		if isRepoConfig {
			expectedTargetRepo := pathElements[0] + "/" + pathElements[1]
			if !isGlobal && len(targetedOrgs) == 0 && len(targetedRepos) == 1 && targetedRepos.Has(expectedTargetRepo) {
				return nil
			}

			errMsg := fmt.Sprintf("config %s is invalid: Must only contain config for repo %s, but", path, expectedTargetRepo)
			var needsAnd bool
			if isGlobal {
				errMsg += " contains global config"
				needsAnd = true
			}
			for _, org := range sets.List(targetedOrgs) {
				errMsg += prefixWithAndIfNeeded(fmt.Sprintf(" contains config for org %s", org), needsAnd)
				needsAnd = true
			}
			for _, repo := range sets.List(targetedRepos.Delete(expectedTargetRepo)) {
				errMsg += prefixWithAndIfNeeded(fmt.Sprintf(" contains config for repo %s", repo), needsAnd)
				needsAnd = true
			}
			errs = append(errs, errors.New(errMsg))
			return nil
		}

		// We should have left the function earlier. Error out so bugs in this code can not be abused.
		return fmt.Errorf("BUG: You should never see this. Path: %s, isGlobal: %t, targetedOrgs: %v, targetedRepos: %v", path, isGlobal, targetedOrgs, targetedRepos)
	}))

	return utilerrors.NewAggregate(errs)
}

func validateUnmanagedBranchprotectionConfigDoesntHaveSubconfig(bp config.BranchProtection) error {
	var errs []error
	if bp.Unmanaged != nil && *bp.Unmanaged {
		if doesUnmanagedBranchprotectionPolicyHaveSettings(bp.Policy) && !bp.HasManagedOrgs() && !bp.HasManagedRepos() && !bp.HasManagedBranches() {
			errs = append(errs, errors.New("branch protection is globally set to unmanaged, but has configuration"))
		}
		for orgName, org := range bp.Orgs {
			// The global level setting is overridden by a lower level
			if org.HasManagedRepos() {
				continue
			}
			for _, repo := range org.Repos {
				if repo.HasManagedBranches() {
					continue
				}
			}
			errs = append(errs, fmt.Errorf("branch protection config is globally set to unmanaged but has configuration for org %s without setting the org to unmanaged: false", orgName))
		}
	}
	for orgName, orgConfig := range bp.Orgs {
		if orgConfig.Unmanaged != nil && *orgConfig.Unmanaged {
			if doesUnmanagedBranchprotectionPolicyHaveSettings(orgConfig.Policy) && !orgConfig.HasManagedRepos() && !orgConfig.HasManagedBranches() {
				errs = append(errs, fmt.Errorf("branch protection config for org %s is set to unmanaged, but it defines settings", orgName))
			}
			for repoName, repo := range orgConfig.Repos {
				// The org level setting is overridden by a lower level
				if repo.HasManagedBranches() {
					continue
				}
				errs = append(errs, fmt.Errorf("branch protection config for repo %s/%s is defined, but branch protection is unmanaged for org %s without setting the repo to unmanaged: false", orgName, repoName, orgName))
			}
		}

		for repoName, repoConfig := range orgConfig.Repos {
			if repoConfig.Unmanaged != nil && *repoConfig.Unmanaged {
				if doesUnmanagedBranchprotectionPolicyHaveSettings(repoConfig.Policy) && !repoConfig.HasManagedBranches() {
					errs = append(errs, fmt.Errorf("branch protection config for repo %s/%s is set to unmanaged, but it defines settings", orgName, repoName))
				}

				for branchName, branch := range repoConfig.Branches {
					// The repo level setting is overridden by a lower level
					if branch.Policy.Managed() {
						continue
					}
					errs = append(errs, fmt.Errorf("branch protection for repo %s/%s is set to unmanaged, but it defines settings for branch %s without setting the branch to unmanaged: false", orgName, repoName, branchName))
				}

			}

			for branchName, branchConfig := range repoConfig.Branches {
				if branchConfig.Unmanaged != nil && *branchConfig.Unmanaged && doesUnmanagedBranchprotectionPolicyHaveSettings(branchConfig.Policy) {
					errs = append(errs, fmt.Errorf("branch protection config for branch %s in repo %s/%s is set to unmanaged but defines settings", branchName, orgName, repoName))
				}
			}
		}
	}

	return utilerrors.NewAggregate(errs)
}

func doesUnmanagedBranchprotectionPolicyHaveSettings(p config.Policy) bool {
	emptyRef := config.Policy{Unmanaged: p.Unmanaged}
	return !reflect.DeepEqual(p, emptyRef)
}

func prefixWithAndIfNeeded(s string, needsAnd bool) string {
	if needsAnd {
		return " and" + s
	}
	return s
}

type hierarchicalConfig interface {
	HasConfigFor() (bool, sets.Set[string], sets.Set[string])
}

func getSupplementalConfigScope(path string, filesystem fs.FS, cfg hierarchicalConfig) (global bool, orgs sets.Set[string], repos sets.Set[string], err error) {
	data, err := fs.ReadFile(filesystem, path)
	if err != nil {
		return false, nil, nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return false, nil, nil, fmt.Errorf("failed to unmarshal %s into %T: %w", path, cfg, err)
	}

	global, orgs, repos = cfg.HasConfigFor()
	return global, orgs, repos, nil
}

type ghAppListingClient interface {
	ListAppInstallations() ([]github.AppInstallation, error)
}

func validateGitHubAppIsInstalled(client ghAppListingClient, allRepos sets.Set[string]) error {
	installations, err := client.ListAppInstallations()
	if err != nil {
		return fmt.Errorf("failed to list app installations from GitHub: %w", err)
	}
	orgsWithInstalledApp := sets.Set[string]{}
	for _, installation := range installations {
		orgsWithInstalledApp.Insert(installation.Account.Login)
	}

	var errs []error
	for _, repo := range sets.List(allRepos) {
		if org := strings.Split(repo, "/")[0]; !orgsWithInstalledApp.Has(org) {
			errs = append(errs, fmt.Errorf("There is configuration for the GitHub org %q but the GitHub app is not installed there", org))
		}
	}

	return utilerrors.NewAggregate(errs)
}

func validateRequiredJobAnnotations(a []string, c config.JobConfig) error {
	validator := func(job config.JobBase, annotations []string) error {
		var errs []error
		for _, annotation := range annotations {
			if _, ok := job.Annotations[annotation]; !ok {
				errs = append(errs, fmt.Errorf(annotation))
			}
		}
		return utilerrors.NewAggregate(errs)
	}
	var errs []error

	for _, presubmits := range c.PresubmitsStatic {
		for _, presubmit := range presubmits {
			if err := validator(presubmit.JobBase, a); err != nil {
				errs = append(errs, fmt.Errorf("job '%s' is missing required annotations: %w", presubmit.Name, err))
			}
		}
	}
	for _, postsubmits := range c.PostsubmitsStatic {
		for _, postsubmit := range postsubmits {
			if err := validator(postsubmit.JobBase, a); err != nil {
				errs = append(errs, fmt.Errorf("job '%s' is missing required annotations: %w", postsubmit.Name, err))
			}
		}
	}
	for _, periodic := range c.Periodics {
		if err := validator(periodic.JobBase, a); err != nil {
			errs = append(errs, fmt.Errorf("job '%s' is missing required annotations: %w", periodic.Name, err))
		}
	}
	return utilerrors.NewAggregate(errs)
}
