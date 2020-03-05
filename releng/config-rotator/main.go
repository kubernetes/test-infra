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
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"sigs.k8s.io/yaml"

	"k8s.io/test-infra/prow/config"
)

type options struct {
	configFile string
	oldVersion string
	newVersion string
}

func cdToRootDir() error {
	if bazelWorkspace := os.Getenv("BUILD_WORKSPACE_DIRECTORY"); bazelWorkspace != "" {
		if err := os.Chdir(bazelWorkspace); err != nil {
			return fmt.Errorf("failed to chdir to bazel workspace (%s): %v", bazelWorkspace, err)
		}
		return nil
	}
	return nil
}

func parseFlags() options {
	o := options{}
	flag.StringVar(&o.configFile, "config-file", "", "Path to the job config")
	flag.StringVar(&o.oldVersion, "old", "", "Old version (beta, stable1, or stable2)")
	flag.StringVar(&o.newVersion, "new", "", "New version (stable1, stable2, or stable3)")
	flag.Parse()
	return o
}

func validateOptions(o options) error {
	if o.configFile == "" {
		return errors.New("--config-file must be specified")
	}
	if o.newVersion == "" {
		return errors.New("--new must be specified")
	}
	if o.oldVersion == "" {
		return errors.New("--old must be specified")
	}
	return nil
}

func updateString(s, old, new string) string {
	return strings.ReplaceAll(s, old, new)
}

func updateJobBase(j *config.JobBase, old, new string) {
	j.Name = updateString(j.Name, old, new)
	for i := range j.Spec.Containers {
		c := &j.Spec.Containers[i]
		for j := range c.Args {
			c.Args[j] = updateString(c.Args[j], old, new)
		}
		for j := range c.Command {
			c.Command[j] = updateString(c.Command[j], old, new)
		}
	}
}

func updateEverything(c *config.JobConfig, old, new string) {
	for _, presubmits := range c.PresubmitsStatic {
		for i := range presubmits {
			updateJobBase(&presubmits[i].JobBase, old, new)
		}
	}
	for _, postsubmits := range c.PostsubmitsStatic {
		for i := range postsubmits {
			updateJobBase(&postsubmits[i].JobBase, old, new)
		}
	}
	for i := range c.Periodics {
		p := &c.Periodics[i]
		updateJobBase(&p.JobBase, old, new)
		for k, v := range p.Annotations {
			if k == "fork-per-release-periodic-interval" {
				f := strings.Fields(v)
				if len(f) > 0 {
					p.Interval = f[0]
					p.Annotations[k] = strings.Join(f[1:], " ")
				}
			}
		}
		for k, v := range p.Annotations {
			if k == "fork-per-release-cron" {
				f := strings.Split(v, ", ")
				if len(f) > 0 {
					p.Cron = f[0]
					p.Annotations[k] = strings.Join(f[1:], ", ")
				}
			}
		}
		for j := range p.Tags {
			p.Tags[j] = updateString(p.Tags[j], old, new)
		}
	}
}

func main() {
	if err := cdToRootDir(); err != nil {
		log.Fatalln(err)
	}
	o := parseFlags()
	if err := validateOptions(o); err != nil {
		log.Fatalln(err)
	}
	c, err := config.ReadJobConfig(o.configFile)
	if err != nil {
		log.Fatalf("Failed to load job config: %v\n", err)
	}
	updateEverything(&c, o.oldVersion, o.newVersion)

	output, err := yaml.Marshal(map[string]interface{}{
		"presubmits":  c.PresubmitsStatic,
		"postsubmits": c.PostsubmitsStatic,
		"periodics":   c.Periodics,
	})
	if err != nil {
		log.Fatalf("Failed to marshal new presubmits: %v\n", err)
	}

	if err := ioutil.WriteFile(o.configFile, output, 0666); err != nil {
		log.Fatalf("Failed to write new presubmits: %v.\n", err)
	}
}
