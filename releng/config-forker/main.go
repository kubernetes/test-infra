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
	"io/ioutil"
	"log"
	"regexp"
	"strings"
	"text/template"

	v1 "k8s.io/api/core/v1"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"sigs.k8s.io/yaml"
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

func generatePostsubmits(c config.JobConfig, version string) (map[string][]config.Postsubmit, error) {
	newPostsubmits := map[string][]config.Postsubmit{}
	for repo, postsubmits := range c.PostsubmitsStatic {
		for _, postsubmit := range postsubmits {
			if postsubmit.Annotations[forkAnnotation] != "true" {
				continue
			}
			p := postsubmit
			p.Name = generateNameVariant(p.Name, version, postsubmit.Annotations[suffixAnnotation] == "true")
			p.SkipBranches = nil
			p.Branches = []string{"release-" + version}
			if p.Spec != nil {
				for i := range p.Spec.Containers {
					c := &p.Spec.Containers[i]
					c.Env = fixEnvVars(c.Env, version)
					c.Image = fixImage(c.Image, version)
					var err error
					c.Args, err = performReplacement(c.Args, version, p.Annotations[replacementAnnotation])
					if err != nil {
						return nil, fmt.Errorf("%s: %v", postsubmit.Name, err)
					}
				}
			}
			p.Annotations = cleanAnnotations(fixTestgridAnnotations(p.Annotations, version, false))
			newPostsubmits[repo] = append(newPostsubmits[repo], p)
		}
	}
	return newPostsubmits, nil
}

func generatePresubmits(c config.JobConfig, version string) (map[string][]config.Presubmit, error) {
	newPresubmits := map[string][]config.Presubmit{}
	for repo, presubmits := range c.PresubmitsStatic {
		for _, presubmit := range presubmits {
			if presubmit.Annotations[forkAnnotation] != "true" {
				continue
			}
			p := presubmit
			p.SkipBranches = nil
			p.Branches = []string{"release-" + version}
			if p.Spec != nil {
				for i := range p.Spec.Containers {
					c := &p.Spec.Containers[i]
					c.Env = fixEnvVars(c.Env, version)
					c.Image = fixImage(c.Image, version)
					var err error
					c.Args, err = performReplacement(c.Args, version, p.Annotations[replacementAnnotation])
					if err != nil {
						return nil, fmt.Errorf("%s: %v", presubmit.Name, err)
					}
				}
			}
			p.Annotations = cleanAnnotations(fixTestgridAnnotations(p.Annotations, version, true))
			newPresubmits[repo] = append(newPresubmits[repo], p)
		}
	}
	return newPresubmits, nil
}

func generatePeriodics(conf config.JobConfig, version string) ([]config.Periodic, error) {
	var newPeriodics []config.Periodic
	for _, periodic := range conf.Periodics {
		if periodic.Annotations[forkAnnotation] != "true" {
			continue
		}
		p := periodic
		p.Name = generateNameVariant(p.Name, version, periodic.Annotations[suffixAnnotation] == "true")
		if p.Spec != nil {
			for i := range p.Spec.Containers {
				c := &p.Spec.Containers[i]
				c.Image = fixImage(c.Image, version)
				c.Env = fixEnvVars(c.Env, version)
				if !config.ShouldDecorate(&conf, p.JobBase.UtilityConfig) {
					c.Command = fixBootstrapArgs(c.Command, version)
					c.Args = fixBootstrapArgs(c.Args, version)
				}
				var err error
				c.Args, err = performReplacement(c.Args, version, p.Annotations[replacementAnnotation])
				if err != nil {
					return nil, fmt.Errorf("%s: %v", periodic.Name, err)
				}
			}
		}
		if config.ShouldDecorate(&conf, p.JobBase.UtilityConfig) {
			p.ExtraRefs = fixExtraRefs(p.ExtraRefs, version)
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
		p.Tags, err = performReplacement(p.Tags, version, p.Annotations[replacementAnnotation])
		if err != nil {
			return nil, fmt.Errorf("%s: %v", periodic.Name, err)
		}
		p.Labels = performDeletion(p.Labels, p.Annotations[deletionAnnotation])
		p.Annotations = cleanAnnotations(fixTestgridAnnotations(p.Annotations, version, false))
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
		return "", fmt.Errorf("failed to parse template %q: %v", s, err)
	}
	wr := bytes.Buffer{}
	err = t.Execute(&wr, c)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %v", err)
	}
	return wr.String(), nil
}

func performReplacement(args []string, version, replacements string) ([]string, error) {
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
		v, err := evaluateTemplate(s[1], struct{ Version string }{version})
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

func fixImage(image, version string) string {
	return strings.ReplaceAll(image, "-master", "-"+version)
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
			break
		case testgridTabNameAnnotation:
			v = strings.ReplaceAll(v, "master", version)
			break
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
	if !strings.HasSuffix(name, "-master") {
		return name + suffix
	}
	return strings.ReplaceAll(name, "-master", suffix)
}

type options struct {
	jobConfig  string
	outputPath string
	newVersion string
}

func parseFlags() options {
	o := options{}
	flag.StringVar(&o.jobConfig, "job-config", "", "Path to the job config")
	flag.StringVar(&o.outputPath, "output", "", "Path to the output yaml. if not specified, just validate.")
	flag.StringVar(&o.newVersion, "version", "", "Version number to generate jobs for")
	flag.Parse()
	return o
}

func validateOptions(o options) error {
	if o.jobConfig == "" {
		return errors.New("--job-config must be specified")
	}
	if o.newVersion == "" {
		return errors.New("--version must be specified")
	}
	if match, err := regexp.MatchString(`^\d+\.\d+$`, o.newVersion); err != nil || !match {
		return fmt.Errorf("%q doesn't look like a valid version number", o.newVersion)
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

	newPresubmits, err := generatePresubmits(c, o.newVersion)
	if err != nil {
		log.Fatalf("Failed to generate presubmits: %v.\n", err)
	}
	newPeriodics, err := generatePeriodics(c, o.newVersion)
	if err != nil {
		log.Fatalf("Failed to generate periodics: %v.\n", err)
	}
	newPostsubmits, err := generatePostsubmits(c, o.newVersion)
	if err != nil {
		log.Fatalf("Failed to generate postsubmits: %v.\n", err)
	}

	output, err := yaml.Marshal(map[string]interface{}{
		"periodics":   newPeriodics,
		"presubmits":  newPresubmits,
		"postsubmits": newPostsubmits,
	})
	if err != nil {
		log.Fatalf("Failed to marshal new presubmits: %v\n", err)
	}

	if o.outputPath != "" {
		if err := ioutil.WriteFile(o.outputPath, output, 0666); err != nil {
			log.Fatalf("Failed to write new presubmits: %v.\n", err)
		}
	} else {
		log.Println("No output file specified, so not writing anything.")
	}
}
