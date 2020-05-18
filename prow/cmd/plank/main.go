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
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	uberzap "go.uber.org/zap"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	ctrlruntimelog "sigs.k8s.io/controller-runtime/pkg/runtime/log"

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

	configPath    string
	jobConfigPath string
	buildCluster  string
	selector      string
	skipReport    bool

	dryRun     bool
	useV2      bool
	kubernetes prowflagutil.KubernetesOptions
	github     prowflagutil.GitHubOptions // TODO(fejta): remove
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.StringVar(&o.totURL, "tot-url", "", "Tot URL")

	fs.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.selector, "label-selector", labels.Everything().String(), "Label selector to be applied in prowjobs. See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors for constructing a label selector.")
	fs.BoolVar(&o.skipReport, "skip-report", false, "Validate that crier is reporting to github, not plank")
	fs.BoolVar(&o.useV2, "use-v2", false, "Experimental: Wether to use the V2 implementation of plank")

	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to GitHub.")
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github} {
		group.AddFlags(fs)
	}

	o.github.AllowDirectAccess = true
	fs.Parse(args)
	return o
}

func (o *options) Validate() error {
	o.github.AllowAnonymous = true
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

	var configAgent config.Agent
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := configAgent.Config

	var logOpts []zap.Opts
	if cfg().LogLevel == "debug" {
		logOpts = append(logOpts, func(o *zap.Options) {
			lvl := uberzap.NewAtomicLevelAt(uberzap.DebugLevel)
			o.Level = &lvl
		})
	}
	ctrlruntimelog.SetLogger(zap.New(logOpts...))

	var reporter func(context.Context)
	if !o.skipReport {
		logrus.Warn("Plank no longer supports github reporting, migrate to crier before June 2020")
		var err error
		reporter, err = deprecatedReporter(o.github, o.kubernetes, o.dryRun, cfg)
		if err != nil {
			logrus.WithError(err).Fatal("Error creating github reporter")
		}
	}

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

	var creator func(options, manager.Manager, config.Getter) error
	if o.useV2 {
		creator = v2Main
	} else {
		creator = v1Main
	}
	if err := creator(o, mgr, cfg); err != nil {
		logrus.WithError(err).Fatal("Error creating plank")
	}

	// Expose prometheus metrics
	metrics.ExposeMetrics("plank", cfg().PushGateway)
	// gather metrics for the jobs handled by plank.
	if reporter != nil {
		interrupts.Run(reporter)
	}
	if err := mgr.Start(interrupts.Context().Done()); err != nil {
		logrus.WithError(err).Fatal("failed to start manager")
	}
}

func v1Main(o options, mgr manager.Manager, cfg config.Getter) error {

	buildClusterClients, err := o.kubernetes.BuildClusterUncachedRuntimeClients(o.dryRun)
	if err != nil {
		return fmt.Errorf("failed to consturct build cluster clients: %w", err)
	}
	c, err := plank.NewController(mgr.GetClient(), buildClusterClients, nil, cfg, o.totURL, o.selector)
	if err != nil {
		return fmt.Errorf("failed to create plank controller: %w", err)
	}

	interrupts.TickLiteral(func() {
		start := time.Now()
		c.SyncMetrics()
		logrus.WithField("metrics-duration", fmt.Sprintf("%v", time.Since(start))).Debug("Metrics synced")
	}, 30*time.Second)
	// run the controller
	if err := mgr.Add(c); err != nil {
		return fmt.Errorf("failed to add controller to manager: %w", err)
	}

	return nil
}

func v2Main(o options, mgr manager.Manager, cfg config.Getter) error {
	buildManagers, err := o.kubernetes.BuildClusterManagers(false,
		func(o *manager.Options) {
			o.Namespace = cfg().PodNamespace
		},
	)
	if err != nil {
		return fmt.Errorf("failed to construct build cluster managers: %w", err)
	}

	for _, buildManager := range buildManagers {
		if err := mgr.Add(buildManager); err != nil {
			return fmt.Errorf("failed to add build manager to main manager: %w", err)
		}
	}

	if err := plank.Add(mgr, buildManagers, cfg, o.totURL, o.selector); err != nil {
		return fmt.Errorf("failed to add plank to mgr: %w", err)
	}

	return nil
}
