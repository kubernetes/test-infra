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

// Package rotator implements the config-rotator tool, which rotates Prow job
// configurations through stability tiers for Kubernetes release branches.
package rotator

import (
	"errors"
	"fmt"
	"os"
	"strings"

	gyaml "go.yaml.in/yaml/v2"
	"sigs.k8s.io/prow/pkg/config"
	"sigs.k8s.io/yaml"
)

// filePermissions is the permission mode used when writing output files.
const filePermissions = 0o600

// Sentinel errors for option validation.
var (
	ErrConfigFileRequired = errors.New("config file must be specified")
	ErrNewVersionRequired = errors.New("new version must be specified")
	ErrOldVersionRequired = errors.New("old version must be specified")
)

// Options holds the configuration for the config-rotator.
type Options struct {
	ConfigFile string
	OldVersion string
	NewVersion string
}

// Validate checks that all required options are set.
func (o Options) Validate() error {
	if o.ConfigFile == "" {
		return ErrConfigFileRequired
	}

	if o.NewVersion == "" {
		return ErrNewVersionRequired
	}

	if o.OldVersion == "" {
		return ErrOldVersionRequired
	}

	return nil
}

// Run executes the config rotation with the given options.
func Run(opts Options) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	jobConfig, err := config.ReadJobConfig(opts.ConfigFile)
	if err != nil {
		return fmt.Errorf("failed to load job config: %w", err)
	}

	updateEverything(&jobConfig, opts.OldVersion, opts.NewVersion)

	// We need to use FutureLineWrap because "fork-per-release-cron" is too long
	// causing the annotation value to be split into two lines.
	gyaml.FutureLineWrap()

	output, err := yaml.Marshal(map[string]any{
		"presubmits":  jobConfig.PresubmitsStatic,
		"postsubmits": jobConfig.PostsubmitsStatic,
		"periodics":   jobConfig.Periodics,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal new presubmits: %w", err)
	}

	if err := os.WriteFile(opts.ConfigFile, output, filePermissions); err != nil {
		return fmt.Errorf("failed to write new presubmits: %w", err)
	}

	return nil
}

func updateString(s, old, replacement string) string {
	return strings.ReplaceAll(s, old, replacement)
}

func updateJobBase(job *config.JobBase, old, replacement string) {
	job.Name = updateString(job.Name, old, replacement)

	for idx := range job.Spec.Containers {
		container := &job.Spec.Containers[idx]

		for argIdx := range container.Args {
			container.Args[argIdx] = updateGenericVersionMarker(container.Args[argIdx])
			container.Args[argIdx] = updateString(container.Args[argIdx], old, replacement)
		}

		for cmdIdx := range container.Command {
			container.Command[cmdIdx] = updateGenericVersionMarker(container.Command[cmdIdx])
			container.Command[cmdIdx] = updateString(container.Command[cmdIdx], old, replacement)
		}
	}
}

func updatePeriodicAnnotations(periodic *config.Periodic, old, replacement string) {
	for key, val := range periodic.Annotations {
		if key == "fork-per-release-periodic-interval" {
			fields := strings.Fields(val)
			if len(fields) > 0 {
				periodic.Interval = fields[0]
				periodic.Annotations[key] = strings.Join(fields[1:], " ")
			}
		}
	}

	for key, val := range periodic.Annotations {
		if key == "fork-per-release-cron" {
			parts := strings.Split(val, ", ")
			if len(parts) > 0 {
				periodic.Cron = parts[0]
				periodic.Annotations[key] = strings.Join(parts[1:], ", ")
			}
		}
	}

	for key, val := range periodic.Annotations {
		if key == "testgrid-tab-name" || key == "testgrid-dashboards" {
			periodic.Annotations[key] = updateString(val, old, replacement)
		}
	}
}

func updatePeriodic(periodic *config.Periodic, old, replacement string) {
	updateJobBase(&periodic.JobBase, old, replacement)
	updatePeriodicAnnotations(periodic, old, replacement)

	for tagIdx := range periodic.Tags {
		periodic.Tags[tagIdx] = updateString(periodic.Tags[tagIdx], old, replacement)
	}
}

func updateEverything(jobConfig *config.JobConfig, old, replacement string) {
	for _, presubmits := range jobConfig.PresubmitsStatic {
		for idx := range presubmits {
			updateJobBase(&presubmits[idx].JobBase, old, replacement)
		}
	}

	for _, postsubmits := range jobConfig.PostsubmitsStatic {
		for idx := range postsubmits {
			updateJobBase(&postsubmits[idx].JobBase, old, replacement)
		}
	}

	for idx := range jobConfig.Periodics {
		updatePeriodic(&jobConfig.Periodics[idx], old, replacement)
	}
}

// Version marker logic

const (
	markerDefault     = "k8s-master"
	markerBeta        = "k8s-beta"
	markerStableOne   = "k8s-stable1"
	markerStableTwo   = "k8s-stable2"
	markerStableThree = "k8s-stable3"
	markerStableFour  = "k8s-stable4"
)

func allowedMarkers() []string {
	return []string{
		markerDefault,
		markerBeta,
		markerStableOne,
		markerStableTwo,
		markerStableThree,
		markerStableFour,
	}
}

func getMarker(s string) string {
	var marker string

	for _, m := range allowedMarkers() {
		if strings.Contains(s, m) {
			marker = m

			break
		}
	}

	return marker
}

func updateGenericVersionMarker(input string) string {
	var newMarker string

	marker := getMarker(input)
	switch marker {
	case markerDefault:
		newMarker = markerBeta
	case markerBeta:
		newMarker = markerStableOne
	case markerStableOne:
		newMarker = markerStableTwo
	case markerStableTwo:
		newMarker = markerStableThree
	case markerStableThree:
		newMarker = markerStableFour
	default:
		newMarker = marker
	}

	return updateString(input, marker, newMarker)
}
