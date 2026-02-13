/*
Copyright 2026 The Kubernetes Authors.

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

// Package forker implements the config-forker tool, which forks Prow job
// configurations for new Kubernetes release branches based on fork-per-release
// annotations.
package forker

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"text/template"

	gyaml "go.yaml.in/yaml/v2"
	v1 "k8s.io/api/core/v1"
	prowapi "sigs.k8s.io/prow/pkg/apis/prowjobs/v1"
	"sigs.k8s.io/prow/pkg/config"
	"sigs.k8s.io/yaml"
)

// Sentinel errors for validation and processing failures.
var (
	ErrJobConfigRequired      = errors.New("job-config must be specified")
	ErrVersionRequired        = errors.New("version must be specified")
	ErrInvalidVersion         = errors.New("invalid version number")
	ErrGoVersionRequired      = errors.New("go-version must be specified")
	ErrInvalidGoVersion       = errors.New("invalid go version")
	ErrConflictingAnnotations = errors.New("conflicting annotations")
	ErrEnvReplacementResult   = errors.New("expected a single string result replacing env var")
	ErrEnvReplacementFormat   = errors.New("expected NAME=VALUE format replacing env var")
	ErrReplacementParse       = errors.New("failed to parse replacement")
)

// Number of parts expected when splitting key=value or replacement pairs.
const replacementParts = 2

// Options configures a forking run.
type Options struct {
	// JobConfig is the path to the prow job config directory.
	JobConfig string
	// OutputPath is where to write the forked YAML. If empty, no output is written.
	OutputPath string
	// Version is the Kubernetes release version (e.g., "1.36").
	Version string
	// GoVersion is the Go version for the release branch (e.g., "1.24.0").
	GoVersion string
}

// Validate checks that the options are well-formed.
func (o Options) Validate() error {
	if o.JobConfig == "" {
		return ErrJobConfigRequired
	}

	if o.Version == "" {
		return ErrVersionRequired
	}

	if match, err := regexp.MatchString(`^\d+\.\d+$`, o.Version); err != nil || !match {
		return fmt.Errorf("%w: %q", ErrInvalidVersion, o.Version)
	}

	if o.GoVersion == "" {
		return ErrGoVersionRequired
	}

	if match, err := regexp.MatchString(
		`^\d+\.\d+(\.\d+)?(rc\d)?$`, o.GoVersion,
	); err != nil || !match {
		return fmt.Errorf(
			"%w: %q should match the format 1.20rc1, 1.20, or 1.20.2",
			ErrInvalidGoVersion, o.GoVersion,
		)
	}

	return nil
}

// Run reads the job config, generates forked jobs, and writes the output YAML.
func Run(opts Options) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	jobConfig, err := config.ReadJobConfig(opts.JobConfig)
	if err != nil {
		return fmt.Errorf("reading job config: %w", err)
	}

	vars := templateVars{
		Version:   opts.Version,
		GoVersion: opts.GoVersion,
	}

	newPresubmits, err := generatePresubmits(jobConfig, vars)
	if err != nil {
		return fmt.Errorf("generating presubmits: %w", err)
	}

	newPeriodics, err := generatePeriodics(jobConfig, vars)
	if err != nil {
		return fmt.Errorf("generating periodics: %w", err)
	}

	newPostsubmits, err := generatePostsubmits(jobConfig, vars)
	if err != nil {
		return fmt.Errorf("generating postsubmits: %w", err)
	}

	gyaml.FutureLineWrap()

	output, err := yaml.Marshal(map[string]any{
		"periodics":   newPeriodics,
		"presubmits":  newPresubmits,
		"postsubmits": newPostsubmits,
	})
	if err != nil {
		return fmt.Errorf("marshaling output: %w", err)
	}

	if opts.OutputPath != "" {
		if err := os.WriteFile(
			opts.OutputPath, output, filePermissions,
		); err != nil {
			return fmt.Errorf("writing output: %w", err)
		}
	}

	return nil
}

const (
	forkAnnotation               = "fork-per-release"
	suffixAnnotation             = "fork-per-release-generic-suffix"
	periodicIntervalAnnotation   = "fork-per-release-periodic-interval"
	cronAnnotation               = "fork-per-release-cron"
	replacementAnnotation        = "fork-per-release-replacements"
	deletionAnnotation           = "fork-per-release-deletions"
	testgridDashboardsAnnotation = "testgrid-dashboards"
	testgridTabNameAnnotation    = "testgrid-tab-name"
	descriptionAnnotation        = "description"
	masterValue                  = "master"
	annotationTrue               = "true"
	kubernetesOrg                = "kubernetes"
)

// filePermissions is the permission mode used when writing output files.
const filePermissions = 0o600

// processContainers applies env var fixes, image fixes, and replacement
// annotations to all containers in a pod spec. It returns an error wrapping
// the job name if any replacement fails.
func processContainers(
	spec *v1.PodSpec,
	vars templateVars,
	annotation string,
	jobName string,
) error {
	if spec == nil {
		return nil
	}

	for idx := range spec.Containers {
		container := &spec.Containers[idx]
		container.Env = fixEnvVars(container.Env, vars.Version)
		container.Image = fixImage(container.Image, vars.Version)

		var err error

		container.Command, err = performReplacement(
			container.Command, vars, annotation,
		)
		if err != nil {
			return fmt.Errorf("%s: %w", jobName, err)
		}

		container.Args, err = performReplacement(
			container.Args, vars, annotation,
		)
		if err != nil {
			return fmt.Errorf("%s: %w", jobName, err)
		}

		for envIdx := range container.Env {
			container.Env[envIdx].Name,
				container.Env[envIdx].Value,
				err = performEnvReplacement(
				container.Env[envIdx].Name,
				container.Env[envIdx].Value,
				vars, annotation,
			)
			if err != nil {
				return fmt.Errorf("%s: %w", jobName, err)
			}
		}
	}

	return nil
}

func generatePostsubmits(
	jobConfig config.JobConfig, vars templateVars,
) (map[string][]config.Postsubmit, error) {
	newPostsubmits := map[string][]config.Postsubmit{}

	for repo, postsubmits := range jobConfig.PostsubmitsStatic {
		for _, postsubmit := range postsubmits {
			if postsubmit.Annotations[forkAnnotation] != annotationTrue {
				continue
			}

			job := postsubmit
			job.Name = generateNameVariant(
				job.Name, vars.Version,
				postsubmit.Annotations[suffixAnnotation] == annotationTrue,
			)
			job.SkipBranches = nil
			job.Branches = []string{"release-" + vars.Version}

			if err := processContainers(
				job.Spec, vars,
				job.Annotations[replacementAnnotation],
				postsubmit.Name,
			); err != nil {
				return nil, err
			}

			job.Annotations = cleanAnnotations(
				fixTestgridAnnotations(
					job.Annotations, vars.Version, false,
				),
			)
			newPostsubmits[repo] = append(
				newPostsubmits[repo], job,
			)
		}
	}

	return newPostsubmits, nil
}

func generatePresubmits(
	jobConfig config.JobConfig, vars templateVars,
) (map[string][]config.Presubmit, error) {
	newPresubmits := map[string][]config.Presubmit{}

	for repo, presubmits := range jobConfig.PresubmitsStatic {
		for _, presubmit := range presubmits {
			if presubmit.Annotations[forkAnnotation] != annotationTrue {
				continue
			}

			job := presubmit
			job.SkipBranches = nil
			job.Branches = []string{"release-" + vars.Version}

			job.Context = generatePresubmitContextVariant(
				job.Name, job.Context, vars.Version,
			)

			if err := processContainers(
				job.Spec, vars,
				job.Annotations[replacementAnnotation],
				presubmit.Name,
			); err != nil {
				return nil, err
			}

			job.Annotations = cleanAnnotations(
				fixTestgridAnnotations(
					job.Annotations, vars.Version, true,
				),
			)
			newPresubmits[repo] = append(
				newPresubmits[repo], job,
			)
		}
	}

	return newPresubmits, nil
}

func shouldDecorate(
	jobConfig *config.JobConfig, util config.UtilityConfig,
) bool {
	if util.Decorate != nil {
		return *util.Decorate
	}

	return jobConfig.DecorateAllJobs
}

func processPeriodicContainers(
	spec *v1.PodSpec,
	conf *config.JobConfig,
	util config.UtilityConfig,
	vars templateVars,
	annotation string,
	jobName string,
) error {
	if spec == nil {
		return nil
	}

	for idx := range spec.Containers {
		container := &spec.Containers[idx]
		container.Image = fixImage(container.Image, vars.Version)
		container.Env = fixEnvVars(container.Env, vars.Version)

		if !shouldDecorate(conf, util) {
			container.Command = fixBootstrapArgs(container.Command, vars.Version)
			container.Args = fixBootstrapArgs(container.Args, vars.Version)
		}

		var err error

		container.Command, err = performReplacement(container.Command, vars, annotation)
		if err != nil {
			return fmt.Errorf("%s: %w", jobName, err)
		}

		container.Args, err = performReplacement(container.Args, vars, annotation)
		if err != nil {
			return fmt.Errorf("%s: %w", jobName, err)
		}

		for envIdx := range container.Env {
			container.Env[envIdx].Name,
				container.Env[envIdx].Value,
				err = performEnvReplacement(
				container.Env[envIdx].Name,
				container.Env[envIdx].Value,
				vars, annotation,
			)
			if err != nil {
				return fmt.Errorf("%s: %w", jobName, err)
			}
		}
	}

	return nil
}

func processPeriodicSchedule(
	job *config.Periodic, periodicName string,
) error {
	if interval, ok := job.Annotations[periodicIntervalAnnotation]; ok {
		if _, ok := job.Annotations[cronAnnotation]; ok {
			return fmt.Errorf(
				"%w: %q specifies both %s and %s",
				ErrConflictingAnnotations,
				periodicName,
				periodicIntervalAnnotation,
				cronAnnotation,
			)
		}

		fields := strings.Fields(interval)
		if len(fields) > 0 {
			job.Interval = fields[0]
			job.Cron = ""
			job.Annotations[periodicIntervalAnnotation] = strings.Join(fields[1:], " ")
		}
	}

	if cron, ok := job.Annotations[cronAnnotation]; ok {
		cronParts := strings.Split(cron, ", ")
		if len(cronParts) > 0 {
			job.Cron = cronParts[0]
			job.Interval = ""
			job.Annotations[cronAnnotation] = strings.Join(cronParts[1:], ", ")
		}
	}

	return nil
}

func generatePeriodics(
	conf config.JobConfig, vars templateVars,
) ([]config.Periodic, error) {
	var newPeriodics []config.Periodic

	for _, periodic := range conf.Periodics {
		if periodic.Annotations[forkAnnotation] != annotationTrue {
			continue
		}

		job := periodic

		job.Name = generateNameVariant(
			job.Name, vars.Version,
			periodic.Annotations[suffixAnnotation] == annotationTrue,
		)

		if err := processPeriodicContainers(
			job.Spec, &conf, job.UtilityConfig, vars,
			job.Annotations[replacementAnnotation], periodic.Name,
		); err != nil {
			return nil, err
		}

		if shouldDecorate(&conf, job.UtilityConfig) {
			job.ExtraRefs = fixExtraRefs(job.ExtraRefs, vars.Version)
		}

		if err := processPeriodicSchedule(&job, periodic.Name); err != nil {
			return nil, err
		}

		var err error

		job.Tags, err = performReplacement(
			job.Tags, vars, job.Annotations[replacementAnnotation],
		)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", periodic.Name, err)
		}

		job.Labels = performDeletion(
			job.Labels, job.Annotations[deletionAnnotation],
		)
		job.Annotations = cleanAnnotations(
			fixTestgridAnnotations(job.Annotations, vars.Version, false),
		)
		newPeriodics = append(newPeriodics, job)
	}

	return newPeriodics, nil
}

func cleanAnnotations(
	annotations map[string]string,
) map[string]string {
	result := map[string]string{}

	for key, val := range annotations {
		if key == forkAnnotation ||
			key == replacementAnnotation ||
			key == deletionAnnotation {
			continue
		}

		if key == periodicIntervalAnnotation && val == "" {
			continue
		}

		if key == cronAnnotation && val == "" {
			continue
		}

		result[key] = val
	}

	return result
}

func evaluateTemplate(
	tmplStr string, data any,
) (string, error) {
	tmpl, err := template.New("t").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf(
			"failed to parse template %q: %w", tmplStr, err,
		)
	}

	buf := bytes.Buffer{}

	err = tmpl.Execute(&buf, data)
	if err != nil {
		return "", fmt.Errorf(
			"failed to execute template: %w", err,
		)
	}

	return buf.String(), nil
}

func performEnvReplacement(
	name, value string,
	vars templateVars,
	replacementStr string,
) (string, string, error) {
	result, err := performReplacement(
		[]string{name + "=" + value}, vars, replacementStr,
	)
	if err != nil {
		return "", "", err
	}

	if len(result) != 1 {
		return "", "", fmt.Errorf(
			"%w, got %d", ErrEnvReplacementResult, len(result),
		)
	}

	parts := strings.SplitN(
		result[0], "=", replacementParts,
	)
	if len(parts) != replacementParts {
		return "", "", fmt.Errorf(
			"%w, got %s", ErrEnvReplacementFormat, result[0],
		)
	}

	return parts[0], parts[1], nil
}

func performReplacement(
	args []string,
	vars templateVars,
	replacementStr string,
) ([]string, error) {
	if args == nil {
		return nil, nil
	}

	if replacementStr == "" {
		return args, nil
	}

	var replacements []string

	allReplacements := strings.SplitSeq(
		replacementStr, ", ",
	)
	for entry := range allReplacements {
		parts := strings.Split(entry, " -> ")
		if len(parts) != replacementParts {
			return nil, fmt.Errorf(
				"%w: %q", ErrReplacementParse, entry,
			)
		}

		val, err := evaluateTemplate(parts[1], vars)
		if err != nil {
			return nil, err
		}

		replacements = append(replacements, parts[0], val)
	}

	replacer := strings.NewReplacer(replacements...)

	newArgs := make([]string, 0, len(args))
	for _, arg := range args {
		newArgs = append(newArgs, replacer.Replace(arg))
	}

	return newArgs, nil
}

func performDeletion(
	args map[string]string, deletions string,
) map[string]string {
	if args == nil {
		return nil
	}

	if deletions == "" {
		return args
	}

	deletionsSet := make(map[string]bool)
	for entry := range strings.SplitSeq(deletions, ", ") {
		deletionsSet[entry] = true
	}

	result := map[string]string{}

	for key, val := range args {
		if !deletionsSet[key] {
			result[key] = val
		}
	}

	return result
}

const masterSuffix = "-master"

func replaceAllMaster(
	str, replacement string,
) string {
	return strings.ReplaceAll(str, masterSuffix, replacement)
}

func fixImage(image, version string) string {
	return replaceAllMaster(image, "-"+version)
}

func fixBootstrapArgs(
	args []string, version string,
) []string {
	if args == nil {
		return nil
	}

	replacer := strings.NewReplacer(
		"--repo=k8s.io/kubernetes="+masterValue,
		"--repo=k8s.io/kubernetes=release-"+version,
		"--repo=k8s.io/kubernetes",
		"--repo=k8s.io/kubernetes=release-"+version,
		"--branch="+masterValue,
		"--branch=release-"+version,
	)

	newArgs := make([]string, 0, len(args))
	for _, arg := range args {
		newArgs = append(newArgs, replacer.Replace(arg))
	}

	return newArgs
}

func fixExtraRefs(
	refs []prowapi.Refs, version string,
) []prowapi.Refs {
	if refs == nil {
		return nil
	}

	newRefs := make([]prowapi.Refs, 0, len(refs))

	for _, ref := range refs {
		if ref.Org == kubernetesOrg &&
			ref.Repo == kubernetesOrg &&
			ref.BaseRef == masterValue {
			ref.BaseRef = "release-" + version
		}

		if ref.Org == kubernetesOrg &&
			ref.Repo == "perf-tests" &&
			ref.BaseRef == masterValue {
			ref.BaseRef = "release-" + version
		}

		newRefs = append(newRefs, ref)
	}

	return newRefs
}

func fixEnvVars(
	vars []v1.EnvVar, version string,
) []v1.EnvVar {
	if vars == nil {
		return nil
	}

	newVars := make([]v1.EnvVar, 0, len(vars))

	for _, envVar := range vars {
		if strings.Contains(
			strings.ToUpper(envVar.Name), "BRANCH",
		) && envVar.Value == masterValue {
			envVar.Value = "release-" + version
		}

		newVars = append(newVars, envVar)
	}

	return newVars
}

func fixTestgridAnnotations(
	annotations map[string]string,
	version string,
	isPresubmit bool,
) map[string]string {
	replacer := strings.NewReplacer(
		masterValue+"-blocking", version+"-blocking",
		masterValue+"-informing", version+"-informing",
	)
	result := map[string]string{}
	didDashboards := false

annotations:
	for key, val := range annotations {
		if isPresubmit {
			// Forked presubmits do not get renamed, and so their
			// annotations will be applied to master. In some cases,
			// they will do things that are so explicitly
			// contradictory the run will fail. Therefore, if we're
			// forking a presubmit, just drop all testgrid config
			// and defer to master.
			if strings.HasPrefix(key, "testgrid-") {
				continue
			}
		}

		switch key {
		case testgridDashboardsAnnotation:
			val = replacer.Replace(val)
			if !inOtherSigReleaseDashboard(val, version) {
				val += ", " + "sig-release-job-config-errors"
			}

			didDashboards = true
		case testgridTabNameAnnotation:
			val = strings.ReplaceAll(
				val, masterValue, version,
			)
		case descriptionAnnotation:
			continue annotations
		}

		result[key] = val
	}

	if !didDashboards && !isPresubmit {
		result[testgridDashboardsAnnotation] = "sig-release-job-config-errors"
	}

	return result
}

func inOtherSigReleaseDashboard(
	existingDashboards, version string,
) bool {
	return strings.Contains(
		existingDashboards, "sig-release-"+version,
	)
}

func generateNameVariant(
	name, version string, generic bool,
) string {
	suffix := "-beta"
	if !generic {
		suffix = "-" + strings.ReplaceAll(version, ".", "-")
	}

	if !strings.HasSuffix(name, masterSuffix) {
		return name + suffix
	}

	return replaceAllMaster(name, suffix)
}

func generatePresubmitContextVariant(
	name, context, version string,
) string {
	suffix := "-" + version

	if context != "" {
		return replaceAllMaster(context, suffix)
	}

	return replaceAllMaster(name, suffix)
}

type templateVars struct {
	Version   string
	GoVersion string
}
