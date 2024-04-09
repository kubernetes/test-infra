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

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"text/template"

	gyaml "gopkg.in/yaml.v2"
	"sigs.k8s.io/yaml"

	v1 "k8s.io/api/core/v1"
	prowapi "sigs.k8s.io/prow/prow/apis/prowjobs/v1"
	"sigs.k8s.io/prow/prow/config"
)

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
)

func generatePostsubmits(c config.JobConfig, vars templateVars) (map[string][]config.Postsubmit, error) {
	newPostsubmits := map[string][]config.Postsubmit{}
	for repo, postsubmits := range c.PostsubmitsStatic {
		for _, postsubmit := range postsubmits {
			if postsubmit.Annotations[forkAnnotation] != "true" {
				continue
			}
			p := postsubmit
			p.Name = generateNameVariant(p.Name, vars.Version, postsubmit.Annotations[suffixAnnotation] == "true")
			p.SkipBranches = nil
			p.Branches = []string{"release-" + vars.Version}
			if p.Spec != nil {
				for i := range p.Spec.Containers {
					c := &p.Spec.Containers[i]
					c.Env = fixEnvVars(c.Env, vars.Version)
					c.Image = fixImage(c.Image, vars.Version)
					var err error
					c.Command, err = performReplacement(c.Command, vars, p.Annotations[replacementAnnotation])
					if err != nil {
						return nil, fmt.Errorf("%s: %w", postsubmit.Name, err)
					}
					c.Args, err = performReplacement(c.Args, vars, p.Annotations[replacementAnnotation])
					if err != nil {
						return nil, fmt.Errorf("%s: %w", postsubmit.Name, err)
					}
					for i := range c.Env {
						c.Env[i].Name, c.Env[i].Value, err = performEnvReplacement(c.Env[i].Name, c.Env[i].Value, vars, p.Annotations[replacementAnnotation])
						if err != nil {
							return nil, fmt.Errorf("%s: %w", postsubmit.Name, err)
						}
					}
				}
			}
			p.Annotations = cleanAnnotations(fixTestgridAnnotations(p.Annotations, vars.Version, false))
			newPostsubmits[repo] = append(newPostsubmits[repo], p)
		}
	}
	return newPostsubmits, nil
}

func generatePresubmits(c config.JobConfig, vars templateVars) (map[string][]config.Presubmit, error) {
	newPresubmits := map[string][]config.Presubmit{}
	for repo, presubmits := range c.PresubmitsStatic {
		for _, presubmit := range presubmits {
			if presubmit.Annotations[forkAnnotation] != "true" {
				continue
			}
			p := presubmit
			p.SkipBranches = nil
			p.Branches = []string{"release-" + vars.Version}
			p.Context = generatePresubmitContextVariant(p.Name, p.Context, vars.Version)
			if p.Spec != nil {
				for i := range p.Spec.Containers {
					c := &p.Spec.Containers[i]
					c.Env = fixEnvVars(c.Env, vars.Version)
					c.Image = fixImage(c.Image, vars.Version)
					var err error
					c.Command, err = performReplacement(c.Command, vars, p.Annotations[replacementAnnotation])
					if err != nil {
						return nil, fmt.Errorf("%s: %w", presubmit.Name, err)
					}
					c.Args, err = performReplacement(c.Args, vars, p.Annotations[replacementAnnotation])
					if err != nil {
						return nil, fmt.Errorf("%s: %w", presubmit.Name, err)
					}
					for i := range c.Env {
						c.Env[i].Name, c.Env[i].Value, err = performEnvReplacement(c.Env[i].Name, c.Env[i].Value, vars, p.Annotations[replacementAnnotation])
						if err != nil {
							return nil, fmt.Errorf("%s: %w", presubmit.Name, err)
						}
					}
				}
			}
			p.Annotations = cleanAnnotations(fixTestgridAnnotations(p.Annotations, vars.Version, true))
			newPresubmits[repo] = append(newPresubmits[repo], p)
		}
	}
	return newPresubmits, nil
}

func shouldDecorate(c *config.JobConfig, util config.UtilityConfig) bool {
	if util.Decorate != nil {
		return *util.Decorate
	}
	return c.DecorateAllJobs
}

func generatePeriodics(conf config.JobConfig, vars templateVars) ([]config.Periodic, error) {
	var newPeriodics []config.Periodic
	for _, periodic := range conf.Periodics {
		if periodic.Annotations[forkAnnotation] != "true" {
			continue
		}
		p := periodic
		p.Name = generateNameVariant(p.Name, vars.Version, periodic.Annotations[suffixAnnotation] == "true")
		if p.Spec != nil {
			for i := range p.Spec.Containers {
				c := &p.Spec.Containers[i]
				c.Image = fixImage(c.Image, vars.Version)
				c.Env = fixEnvVars(c.Env, vars.Version)
				if !shouldDecorate(&conf, p.JobBase.UtilityConfig) {
					c.Command = fixBootstrapArgs(c.Command, vars.Version)
					c.Args = fixBootstrapArgs(c.Args, vars.Version)
				}
				var err error
				c.Command, err = performReplacement(c.Command, vars, p.Annotations[replacementAnnotation])
				if err != nil {
					return nil, fmt.Errorf("%s: %w", periodic.Name, err)
				}
				c.Args, err = performReplacement(c.Args, vars, p.Annotations[replacementAnnotation])
				if err != nil {
					return nil, fmt.Errorf("%s: %w", periodic.Name, err)
				}
				for i := range c.Env {
					c.Env[i].Name, c.Env[i].Value, err = performEnvReplacement(c.Env[i].Name, c.Env[i].Value, vars, p.Annotations[replacementAnnotation])
					if err != nil {
						return nil, fmt.Errorf("%s: %w", periodic.Name, err)
					}
				}
			}
		}
		if shouldDecorate(&conf, p.JobBase.UtilityConfig) {
			p.ExtraRefs = fixExtraRefs(p.ExtraRefs, vars.Version)
		}
		if interval, ok := p.Annotations[periodicIntervalAnnotation]; ok {
			if _, ok := p.Annotations[cronAnnotation]; ok {
				return nil, fmt.Errorf("%q specifies both %s and %s, which is illegal", periodic.Name, periodicIntervalAnnotation, cronAnnotation)
			}
			f := strings.Fields(interval)
			if len(f) > 0 {
				p.Interval = f[0]
				p.Cron = ""
				p.Annotations[periodicIntervalAnnotation] = strings.Join(f[1:], " ")
			}
		}
		if cron, ok := p.Annotations[cronAnnotation]; ok {
			c := strings.Split(cron, ", ")
			if len(c) > 0 {
				p.Cron = c[0]
				p.Interval = ""
				p.Annotations[cronAnnotation] = strings.Join(c[1:], ", ")
			}
		}
		var err error
		p.Tags, err = performReplacement(p.Tags, vars, p.Annotations[replacementAnnotation])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", periodic.Name, err)
		}
		p.Labels = performDeletion(p.Labels, p.Annotations[deletionAnnotation])
		p.Annotations = cleanAnnotations(fixTestgridAnnotations(p.Annotations, vars.Version, false))
		newPeriodics = append(newPeriodics, p)
	}
	return newPeriodics, nil
}

func cleanAnnotations(annotations map[string]string) map[string]string {
	result := map[string]string{}
	for k, v := range annotations {
		if k == forkAnnotation || k == replacementAnnotation || k == deletionAnnotation {
			continue
		}
		if k == periodicIntervalAnnotation && v == "" {
			continue
		}
		if k == cronAnnotation && v == "" {
			continue
		}
		result[k] = v
	}
	return result
}

func evaluateTemplate(s string, c interface{}) (string, error) {
	t, err := template.New("t").Parse(s)
	if err != nil {
		return "", fmt.Errorf("failed to parse template %q: %w", s, err)
	}
	wr := bytes.Buffer{}
	err = t.Execute(&wr, c)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}
	return wr.String(), nil
}

func performEnvReplacement(name, value string, vars templateVars, replacements string) (string, string, error) {
	v, err := performReplacement([]string{name + "=" + value}, vars, replacements)
	if err != nil {
		return "", "", err
	}
	if len(v) != 1 {
		return "", "", fmt.Errorf("expected a single string result replacing env var, got %d", len(v))
	}
	parts := strings.SplitN(v[0], "=", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected NAME=VALUE format replacing env var, got %s", v[0])
	}
	return parts[0], parts[1], nil
}

func performReplacement(args []string, vars templateVars, replacements string) ([]string, error) {
	if args == nil {
		return nil, nil
	}
	if replacements == "" {
		return args, nil
	}

	var rs []string
	as := strings.Split(replacements, ", ")
	for _, r := range as {
		s := strings.Split(r, " -> ")
		if len(s) != 2 {
			return nil, fmt.Errorf("failed to parse replacement %q", r)
		}
		v, err := evaluateTemplate(s[1], vars)
		if err != nil {
			return nil, err
		}
		rs = append(rs, s[0], v)
	}
	replacer := strings.NewReplacer(rs...)

	newArgs := make([]string, 0, len(args))
	for _, a := range args {
		newArgs = append(newArgs, replacer.Replace(a))
	}

	return newArgs, nil
}

func performDeletion(args map[string]string, deletions string) map[string]string {
	if args == nil {
		return nil
	}
	if deletions == "" {
		return args
	}

	deletionsSet := make(map[string]bool)
	for _, s := range strings.Split(deletions, ", ") {
		deletionsSet[s] = true
	}

	result := map[string]string{}

	for k, v := range args {
		if !deletionsSet[k] {
			result[k] = v
		}
	}

	return result
}

const masterSuffix = "-master"

func replaceAllMaster(s, new string) string {
	return strings.ReplaceAll(s, masterSuffix, new)
}

func fixImage(image, version string) string {
	return replaceAllMaster(image, "-"+version)
}

func fixBootstrapArgs(args []string, version string) []string {
	if args == nil {
		return nil
	}
	replacer := strings.NewReplacer(
		"--repo=k8s.io/kubernetes=master", "--repo=k8s.io/kubernetes=release-"+version,
		"--repo=k8s.io/kubernetes", "--repo=k8s.io/kubernetes=release-"+version,
		"--branch=master", "--branch=release-"+version,
	)
	newArgs := make([]string, 0, len(args))
	for _, arg := range args {
		newArgs = append(newArgs, replacer.Replace(arg))
	}
	return newArgs
}

func fixExtraRefs(refs []prowapi.Refs, version string) []prowapi.Refs {
	if refs == nil {
		return nil
	}
	newRefs := make([]prowapi.Refs, 0, len(refs))
	for _, r := range refs {
		if r.Org == "kubernetes" && r.Repo == "kubernetes" && r.BaseRef == "master" {
			r.BaseRef = "release-" + version
		}
		if r.Org == "kubernetes" && r.Repo == "perf-tests" && r.BaseRef == "master" {
			r.BaseRef = "release-" + version
		}
		newRefs = append(newRefs, r)
	}
	return newRefs
}

func fixEnvVars(vars []v1.EnvVar, version string) []v1.EnvVar {
	if vars == nil {
		return nil
	}
	newVars := make([]v1.EnvVar, 0, len(vars))
	for _, v := range vars {
		if strings.Contains(strings.ToUpper(v.Name), "BRANCH") && v.Value == "master" {
			v.Value = "release-" + version
		}
		newVars = append(newVars, v)
	}
	return newVars
}

func fixTestgridAnnotations(annotations map[string]string, version string, isPresubmit bool) map[string]string {
	r := strings.NewReplacer(
		"master-blocking", version+"-blocking",
		"master-informing", version+"-informing",
	)
	a := map[string]string{}
	didDashboards := false
annotations:
	for k, v := range annotations {
		if isPresubmit {
			// Forked presubmits do not get renamed, and so their annotations will be applied to master.
			// In some cases, they will do things that are so explicitly contradictory the run will fail.
			// Therefore, if we're forking a presubmit, just drop all testgrid config and defer to master.
			if strings.HasPrefix(k, "testgrid-") {
				continue
			}
		}
		switch k {
		case testgridDashboardsAnnotation:
			fmt.Println(v)
			v = r.Replace(v)
			if !inOtherSigReleaseDashboard(v, version) {
				v += ", " + "sig-release-job-config-errors"
			}
			didDashboards = true
		case testgridTabNameAnnotation:
			v = strings.ReplaceAll(v, "master", version)
		case descriptionAnnotation:
			continue annotations
		}
		a[k] = v
	}
	if !didDashboards && !isPresubmit {
		a[testgridDashboardsAnnotation] = "sig-release-job-config-errors"
	}
	return a

}

func inOtherSigReleaseDashboard(existingDashboards, version string) bool {
	return strings.Contains(existingDashboards, "sig-release-"+version)
}

func generateNameVariant(name, version string, generic bool) string {
	suffix := "-beta"
	if !generic {
		suffix = "-" + strings.ReplaceAll(version, ".", "-")
	}
	if !strings.HasSuffix(name, masterSuffix) {
		return name + suffix
	}
	return replaceAllMaster(name, suffix)
}

func generatePresubmitContextVariant(name, context, version string) string {
	suffix := "-" + version

	if context != "" {
		return replaceAllMaster(context, suffix)
	}
	return replaceAllMaster(name, suffix)
}

type options struct {
	jobConfig  string
	outputPath string
	vars       templateVars
}

type templateVars struct {
	Version   string
	GoVersion string
}

func parseFlags() options {
	o := options{}
	flag.StringVar(&o.jobConfig, "job-config", "", "Path to the job config")
	flag.StringVar(&o.outputPath, "output", "", "Path to the output yaml. if not specified, just validate.")
	flag.StringVar(&o.vars.Version, "version", "", "Version number to generate jobs for")
	flag.StringVar(&o.vars.GoVersion, "go-version", "", "Current go version in use; see http://git.k8s.io/kubernetes/.go-version")
	flag.Parse()
	return o
}

func validateOptions(o options) error {
	if o.jobConfig == "" {
		return errors.New("--job-config must be specified")
	}
	if o.vars.Version == "" {
		return errors.New("--version must be specified")
	}
	if match, err := regexp.MatchString(`^\d+\.\d+$`, o.vars.Version); err != nil || !match {
		return fmt.Errorf("%q doesn't look like a valid version number", o.vars.Version)
	}
	if o.vars.GoVersion == "" {
		return errors.New("--go-version must be specified; http://git.k8s.io/kubernetes/.go-version contains the recommended value")
	}
	if match, err := regexp.MatchString(`^\d+\.\d+(\.\d+)?(rc\d)?$`, o.vars.GoVersion); err != nil || !match {
		return fmt.Errorf("%q doesn't look like a valid go version; should match the format 1.20rc1, 1.20, or 1.20.2", o.vars.GoVersion)
	}
	return nil
}

func main() {
	o := parseFlags()
	if err := validateOptions(o); err != nil {
		log.Fatalln(err)
	}
	c, err := config.ReadJobConfig(o.jobConfig)
	if err != nil {
		log.Fatalf("Failed to load job config: %v\n", err)
	}

	newPresubmits, err := generatePresubmits(c, o.vars)
	if err != nil {
		log.Fatalf("Failed to generate presubmits: %v.\n", err)
	}
	newPeriodics, err := generatePeriodics(c, o.vars)
	if err != nil {
		log.Fatalf("Failed to generate periodics: %v.\n", err)
	}
	newPostsubmits, err := generatePostsubmits(c, o.vars)
	if err != nil {
		log.Fatalf("Failed to generate postsubmits: %v.\n", err)
	}

	// We need to use FutureLineWrap because "fork-per-release-cron" is too long
	// causing the annotation value to be split into two lines.
	// We use gopkg.in/yaml here because sigs.k8s.io/yaml doesn't export this
	// function. sigs.k8s.io/yaml uses gopkg.in/yaml under the hood.
	gyaml.FutureLineWrap()

	output, err := yaml.Marshal(map[string]interface{}{
		"periodics":   newPeriodics,
		"presubmits":  newPresubmits,
		"postsubmits": newPostsubmits,
	})
	if err != nil {
		log.Fatalf("Failed to marshal new presubmits: %v\n", err)
	}

	if o.outputPath != "" {
		if err := os.WriteFile(o.outputPath, output, 0666); err != nil {
			log.Fatalf("Failed to write new presubmits: %v.\n", err)
		}
	} else {
		log.Println("No output file specified, so not writing anything.")
	}
}
