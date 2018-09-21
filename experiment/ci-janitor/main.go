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
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	"k8s.io/test-infra/prow/flagutil"
)

type options struct {
	config      flagutil.ConfigOptions
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
		"k8s-gke-gpu-pr",
		"k8s-presubmit-scale",
	}
)

func (o *options) Validate() error {
	if err := o.config.Validate(false); err != nil {
		return err
	}

	if o.janitorPath == "" {
		return errors.New("required flag --janitor-path was unset")
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.StringVar(&o.janitorPath, "janitor-path", "", "Path to janitor.py.")
	o.config.AddFlags(fs)
	fs.Parse(os.Args[1:])
	return o
}

func findProject(spec *v1.PodSpec) (string, int) {
	project := ""
	ttl := defaultTTL
	for _, container := range spec.Containers {
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

	configAgent, err := o.config.Agent()
	if err != nil {
		logrus.WithError(err).Fatal("Error loading config.")
	}
	conf := configAgent.Config()

	failed := []string{}

	for _, v := range conf.AllPresubmits(nil) {
		if project, ttl := findProject(v.Spec); project != "" {
			if err := clean(project, o.janitorPath, ttl); err != nil {
				failed = append(failed, project)
			}
		}
	}

	for _, v := range conf.AllPostsubmits(nil) {
		if project, ttl := findProject(v.Spec); project != "" {
			if err := clean(project, o.janitorPath, ttl); err != nil {
				failed = append(failed, project)
			}
		}
	}

	for _, v := range conf.AllPeriodics() {
		if project, ttl := findProject(v.Spec); project != "" {
			if err := clean(project, o.janitorPath, ttl); err != nil {
				failed = append(failed, project)
			}
		}
	}

	if len(failed) == 0 {
		logrus.Info("Successfully cleaned up all projects!")
		os.Exit(0)
	}

	logrus.Warnf("Failed clean %d projects: %v", len(failed), failed)
	os.Exit(1)
}
