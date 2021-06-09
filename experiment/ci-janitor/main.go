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

// ci-janitor cleans up dedicated projects in k8s prowjob configs
package main

import (
	"errors"
	"flag"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/test-infra/prow/config"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
)

type options struct {
	prowConfig  configflagutil.ConfigOptions
	janitorPath string
}

var (
	defaultTTL = 24
	soakTTL    = 24 * 10
	blocked    = []string{
		"kubernetes-scale",  // Let it's up/down job handle the resources
		"k8s-scale-testing", // As it can be running some manual experiments
		// PR projects, migrate to boskos!
		"k8s-jkns-pr-gce",
		"k8s-jkns-pr-gce-bazel",
		"k8s-jkns-pr-gce-etcd3",
		"k8s-jkns-pr-gci-gce",
		"k8s-jkns-pr-gci-gke",
		"k8s-jkns-pr-gci-kubemark",
		"k8s-jkns-pr-gke",
		"k8s-jkns-pr-kubeadm",
		"k8s-jkns-pr-kubemark",
		"k8s-jkns-pr-node-e2e",
		"k8s-jkns-pr-gce-gpus",
		"k8s-presubmit-scale",
	}
)

func (o *options) Validate() error {
	if err := o.prowConfig.Validate(false); err != nil {
		return err
	}

	if o.janitorPath == "" {
		return errors.New("required flag --janitor-path was unset")
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	o.prowConfig.AddFlags(flag.CommandLine)
	flag.StringVar(&o.janitorPath, "janitor-path", "", "Path to gcp_janitor.py.")
	flag.Parse()
	return o
}

func containers(jb config.JobBase) []v1.Container {
	var containers []v1.Container
	if jb.Spec != nil {
		containers = append(containers, jb.Spec.Containers...)
		containers = append(containers, jb.Spec.InitContainers...)
	}
	return containers
}

func findProject(jb config.JobBase) (string, int) {
	project := ""
	ttl := defaultTTL
	for _, container := range containers(jb) {
		for _, arg := range container.Args {
			if strings.HasPrefix(arg, "--gcp-project=") {
				project = strings.TrimPrefix(arg, "--gcp-project=")
			}

			if arg == "--soak" {
				ttl = soakTTL
			}
		}
	}

	return project, ttl
}

func clean(proj, janitorPath string, ttl int) error {
	for _, bad := range blocked {
		if bad == proj {
			logrus.Infof("Will skip project %s", proj)
			return nil
		}
	}

	logrus.Infof("Will clean up %s with ttl %d h", proj, ttl)

	cmd := exec.Command(janitorPath, fmt.Sprintf("--project=%s", proj), fmt.Sprintf("--hour=%d", ttl))
	b, err := cmd.CombinedOutput()
	if err != nil {
		logrus.WithError(err).Errorf("failed to clean up project %s, error info: %s", proj, string(b))
	} else {
		logrus.Infof("successfully cleaned up project %s", proj)
	}

	return err
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	agent, err := o.prowConfig.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error loading config.")
	}
	conf := agent.Config()

	failed := []string{}

	var jobs []config.JobBase

	for _, v := range conf.AllStaticPresubmits(nil) {
		jobs = append(jobs, v.JobBase)
	}
	for _, v := range conf.AllStaticPostsubmits(nil) {
		jobs = append(jobs, v.JobBase)
	}
	for _, v := range conf.AllPeriodics() {
		jobs = append(jobs, v.JobBase)
	}

	for _, j := range jobs {
		if project, ttl := findProject(j); project != "" {
			if err := clean(project, o.janitorPath, ttl); err != nil {
				logrus.WithError(err).Errorf("failed to clean %q", project)
				failed = append(failed, project)
			}
		}
	}

	if len(failed) > 0 {
		logrus.Fatalf("Failed clean %d projects: %v", len(failed), failed)
	}

	logrus.Info("Successfully cleaned up all projects!")
}
