/*
Copyright 2017 The Kubernetes Authors.

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
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/pjutil"

	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/secret"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/plank"
)

type options struct {
	totURL string

	configPath    string
	jobConfigPath string
	buildCluster  string
	selector      string
	skipReport    bool

	dryRun     bool
	kubernetes prowflagutil.KubernetesOptions
	github     prowflagutil.GitHubOptions
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.StringVar(&o.totURL, "tot-url", "", "Tot URL")

	fs.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.selector, "label-selector", labels.Everything().String(), "Label selector to be applied in prowjobs. See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors for constructing a label selector.")
	fs.BoolVar(&o.skipReport, "skip-report", false, "Whether or not to ignore report with githubClient")

	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to GitHub.")
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github} {
		group.AddFlags(fs)
	}

	fs.Parse(args)
	o.configPath = config.ConfigPath(o.configPath)
	return o
}

func (o *options) Validate() error {
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github} {
		if err := group.Validate(o.dryRun); err != nil {
			return err
		}
	}

	if _, err := labels.Parse(o.selector); err != nil {
		return fmt.Errorf("parse label selector: %v", err)
	}

	return nil
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	defer interrupts.WaitForGracefulShutdown()

	pjutil.ServePProf()

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := configAgent.Config

	secretAgent := &secret.Agent{}
	if o.github.TokenPath != "" {
		if err := secretAgent.Start([]string{o.github.TokenPath}); err != nil {
			logrus.WithError(err).Fatal("Error starting secrets agent.")
		}
	}

	githubClient, err := o.github.GitHubClient(secretAgent, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}

	prowJobClient, err := o.kubernetes.ProwJobClient(cfg().ProwJobNamespace, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting prowjob client.")
	}

	buildClusterClients, err := o.kubernetes.BuildClusterClients(cfg().PodNamespace, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating build cluster clients.")
	}

	c, err := plank.NewController(prowJobClient, buildClusterClients, githubClient, nil, cfg, o.totURL, o.selector, o.skipReport)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating plank controller.")
	}

	// Expose prometheus metrics
	metrics.ExposeMetrics("plank", cfg().PushGateway)
	// gather metrics for the jobs handled by plank.
	interrupts.TickLiteral(func() {
		start := time.Now()
		c.SyncMetrics()
		logrus.WithField("metrics-duration", fmt.Sprintf("%v", time.Since(start))).Debug("Metrics synced")
	}, 30*time.Second)

	// run the controller
	interrupts.TickLiteral(func() {
		start := time.Now()
		if err := c.Sync(); err != nil {
			logrus.WithError(err).Error("Error syncing.")
		}
		logrus.WithField("duration", fmt.Sprintf("%v", time.Since(start))).Info("Synced")
	}, 30*time.Second)
}
