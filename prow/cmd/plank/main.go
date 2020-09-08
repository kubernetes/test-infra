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
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/config"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/plank"
)

type options struct {
	totURL string

	configPath           string
	jobConfigPath        string
	selector             string
	deprecatedSkipReport bool

	dryRun                 bool
	kubernetes             prowflagutil.KubernetesOptions
	instrumentationOptions prowflagutil.InstrumentationOptions
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.StringVar(&o.totURL, "tot-url", "", "Tot URL")

	fs.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.selector, "label-selector", labels.Everything().String(), "Label selector to be applied in prowjobs. See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors for constructing a label selector.")
	fs.BoolVar(&o.deprecatedSkipReport, "skip-report", false, "No-Op flag kept for compatibility. Will be removed in September 2020.")

	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to GitHub.")
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.instrumentationOptions} {
		group.AddFlags(fs)
	}

	fs.Parse(args)
	return o
}

func (o *options) Validate() error {
	for _, group := range []flagutil.OptionGroup{&o.kubernetes} {
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

	if o.deprecatedSkipReport {
		logrus.Warning("The deprecated --skip-report flag has been set. It doesn't do anything anymore and will be removed in September 2020.")
	}

	defer interrupts.WaitForGracefulShutdown()

	pjutil.ServePProf(o.instrumentationOptions.PProfPort)

	var configAgent config.Agent
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := configAgent.Config

	infrastructureClusterConfig, err := o.kubernetes.InfrastructureClusterConfig(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting infrastructure cluster config.")
	}
	opts := manager.Options{
		MetricsBindAddress: "0",
		Namespace:          cfg().ProwJobNamespace,
	}
	mgr, err := manager.New(infrastructureClusterConfig, opts)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating manager")
	}

	buildClusterClients, err := o.kubernetes.BuildClusterUncachedRuntimeClients(o.dryRun)
	if err != nil {
		logrus.WithError(err).Error("Error creating build cluster clients. Is there a bad entry in the kubeconfig secret?")
	}

	c, err := plank.NewController(mgr.GetClient(), buildClusterClients, nil, cfg, o.totURL, o.selector)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating plank controller.")
	}

	// Expose prometheus metrics
	metrics.ExposeMetrics("plank", cfg().PushGateway, o.instrumentationOptions.MetricsPort)
	// gather metrics for the jobs handled by plank.
	interrupts.TickLiteral(func() {
		start := time.Now()
		c.SyncMetrics()
		logrus.WithField("metrics-duration", fmt.Sprintf("%v", time.Since(start))).Debug("Metrics synced")
	}, 30*time.Second)

	// run the controller
	if err := mgr.Add(c); err != nil {
		logrus.WithError(err).Fatal("failed to add controller to manager")
	}
	if err := mgr.Start(interrupts.Context().Done()); err != nil {
		logrus.WithError(err).Fatal("failed to start manager")
	}
}
