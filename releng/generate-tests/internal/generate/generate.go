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

// Package generate creates Prow periodic job and TestGrid test group
// configurations from version tiers and static test suite config.
package generate

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"k8s.io/test-infra/releng/generate-tests/internal/config"
	"k8s.io/test-infra/releng/generate-tests/internal/types"
	"k8s.io/test-infra/releng/generate-tests/internal/version"
)

const (
	// splitKeyValueParts is the number of parts when splitting key=value args.
	splitKeyValueParts = 2

	// decorationTimeoutBuffer is the additional minutes added to suite timeout for decoration timeout.
	decorationTimeoutBuffer = 20
)

// ApplyOverrides removes existing args matching the override name prefix and
// appends the overrides. This matches the Python behavior: remove the original,
// then append the override at the end.
func ApplyOverrides(args, overrides []string) []string {
	for _, override := range overrides {
		name := strings.SplitN(override, "=", splitKeyValueParts)[0]

		filtered := make([]string, 0, len(args))
		for _, arg := range args {
			argName := strings.SplitN(strings.TrimSpace(arg), "=", splitKeyValueParts)[0]
			if argName != name {
				filtered = append(filtered, arg)
			}
		}

		args = filtered
	}

	result := make([]string, 0, len(args)+len(overrides))
	result = append(result, args...)
	result = append(result, overrides...)

	return result
}

func buildAnnotations(
	jobCfg config.JobConfig, tier version.Tier, suiteName string,
) map[string]string {
	var dashboard string

	switch {
	case jobCfg.ReleaseBlocking:
		dashboard = fmt.Sprintf("sig-release-%s-blocking", tier.Version.String())
	case jobCfg.ReleaseInforming:
		dashboard = fmt.Sprintf("sig-release-%s-informing", tier.Version.String())
	default:
		dashboard = "sig-release-generated"
	}

	annotations := map[string]string{
		"testgrid-tab-name":   fmt.Sprintf("gce-cos-k8s%s-%s", tier.Marker, suiteName),
		"testgrid-dashboards": dashboard,
	}

	numFailures := jobCfg.NumFailuresToAlert
	if tier.Marker == "stable4" && suiteName == "default" {
		numFailures = config.NumFailuresToAlert
	}

	if numFailures > 0 {
		annotations["testgrid-num-failures-to-alert"] = strconv.Itoa(numFailures)
	}

	return annotations
}

// Jobs creates all periodic job configs and testgrid configs from the given
// version tiers.
func Jobs(tiers []version.Tier) (types.ProwConfig, types.TestGridConfig) {
	suites := config.TestSuites()
	jobConfigs := config.Jobs()

	type jobEntry struct {
		name string
		job  types.ProwJob
		tg   types.TestGroup
	}

	entries := make([]jobEntry, 0, len(tiers)*len(jobConfigs))

	for _, tier := range tiers {
		for _, jobCfg := range jobConfigs {
			suiteName := jobCfg.Suite

			suiteConfigName := suiteName
			if jobCfg.TestSuiteOverride != "" {
				suiteConfigName = jobCfg.TestSuiteOverride
			}

			suite := suites[suiteConfigName]
			jobName := fmt.Sprintf("ci-kubernetes-e2e-gce-cos-k8s%s-%s", tier.Marker, suiteName)
			containerArgs := buildContainerArgs(suite, jobCfg, tier)
			annotations := buildAnnotations(jobCfg, tier, suiteName)

			prowJob := buildProwJob(jobName, tier, suite, containerArgs, annotations)
			testGroup := buildTestGroup(jobName)

			entries = append(entries, jobEntry{name: jobName, job: prowJob, tg: testGroup})
		}
	}

	// Sort by job name for deterministic output.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	prowJobs := make([]types.ProwJob, 0, len(entries))
	testGroups := make([]types.TestGroup, 0, len(entries))

	for _, entry := range entries {
		prowJobs = append(prowJobs, entry.job)
		testGroups = append(testGroups, entry.tg)
	}

	return types.ProwConfig{Periodics: prowJobs}, types.TestGridConfig{TestGroups: testGroups}
}

func buildContainerArgs(
	suite config.TestSuiteConfig, jobCfg config.JobConfig, tier version.Tier,
) []string {
	var containerArgs []string

	containerArgs = append(containerArgs, config.CloudProviderArgs()...)
	containerArgs = append(containerArgs, config.ImageArgs()...)
	containerArgs = append(containerArgs, version.Args(tier.Version)...)
	containerArgs = append(containerArgs, suite.Args...)

	if len(jobCfg.OverrideArgs) > 0 {
		containerArgs = ApplyOverrides(containerArgs, jobCfg.OverrideArgs)
	}

	return containerArgs
}

func buildProwJob(
	jobName string,
	tier version.Tier,
	suite config.TestSuiteConfig,
	containerArgs []string,
	annotations map[string]string,
) types.ProwJob {
	resources := types.ProwResources{
		Requests: suite.Resources,
		Limits:   suite.Resources,
	}

	return types.ProwJob{
		Tags:     []string{"generated"},
		Interval: tier.Interval,
		Labels: map[string]string{
			"preset-service-account": "true",
			"preset-k8s-ssh":         "true",
		},
		Decorate: true,
		DecorationConfig: map[string]string{
			"timeout": fmt.Sprintf("%dm", suite.Timeout+decorationTimeoutBuffer),
		},
		Name: jobName,
		Spec: types.ProwSpec{
			Containers: []types.ProwContainer{
				{
					Command: []string{
						"runner.sh",
						"/workspace/scenarios/kubernetes_e2e.py",
					},
					Args:      containerArgs,
					Image:     config.DefaultImage,
					Resources: resources,
				},
			},
		},
		Cluster: suite.Cluster,
		ExtraRefs: []types.ProwExtraRef{
			{
				Org:       "kubernetes",
				Repo:      "kubernetes",
				BaseRef:   "release-" + tier.Version.String(),
				PathAlias: "k8s.io/kubernetes",
			},
		},
		Annotations: annotations,
	}
}

func buildTestGroup(jobName string) types.TestGroup {
	return types.TestGroup{
		Name:      jobName,
		GCSPrefix: config.GCSLogPrefix + jobName,
		ColumnHeader: []types.ColumnHeader{
			{ConfigurationValue: "node_os_image"},
			{ConfigurationValue: "master_os_image"},
			{ConfigurationValue: "Commit"},
			{ConfigurationValue: "infra-commit"},
		},
	}
}
