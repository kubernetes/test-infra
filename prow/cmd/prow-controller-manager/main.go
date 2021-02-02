/*
Copyright 2020 The Kubernetes Authors.

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
	"os"

	"github.com/sirupsen/logrus"
	uberzap "go.uber.org/zap"
	"k8s.io/apimachinery/pkg/labels"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlruntimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
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

var allControllers = sets.NewString(plank.ControllerName)

type options struct {
	totURL string

	configPath              string
	jobConfigPath           string
	buildCluster            string
	selector                string
	leaderElectionNamespace string
	enabledControllers      prowflagutil.Strings

	dryRun                 bool
	useV2                  bool
	kubernetes             prowflagutil.KubernetesOptions
	github                 prowflagutil.GitHubOptions // TODO(fejta): remove
	instrumentationOptions prowflagutil.InstrumentationOptions
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	o.enabledControllers = prowflagutil.NewStrings(allControllers.List()...)
	fs.StringVar(&o.totURL, "tot-url", "", "Tot URL")

	fs.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.selector, "label-selector", labels.Everything().String(), "Label selector to be applied in prowjobs. See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors for constructing a label selector.")
	fs.Var(&o.enabledControllers, "enable-controller", fmt.Sprintf("Controllers to enable. Can be passed multiple times. Defaults to all controllers (%v)", allControllers.List()))

	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to GitHub.")
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github, &o.instrumentationOptions} {
		group.AddFlags(fs)
	}

	fs.Parse(args)
	return o
}

func (o *options) Validate() error {
	o.github.AllowAnonymous = true

	var errs []error
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github} {
		if err := group.Validate(o.dryRun); err != nil {
			errs = append(errs, err)
		}
	}

	for _, enabledController := range o.enabledControllers.Strings() {
		if !allControllers.Has(enabledController) {
			errs = append(errs, fmt.Errorf("unknown controller %s was configured via --enabled-controller", enabledController))
		}
	}

	if n := len(allControllers.Intersection(sets.NewString(o.enabledControllers.Strings()...))); n == 0 {
		errs = append(errs, errors.New("no controllers configured"))
	}

	if _, err := labels.Parse(o.selector); err != nil {
		errs = append(errs, fmt.Errorf("parse label selector: %w", err))
	}

	return utilerrors.NewAggregate(errs)
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	defer interrupts.WaitForGracefulShutdown()

	pjutil.ServePProf(o.instrumentationOptions.PProfPort)

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

	infrastructureClusterConfig, err := o.kubernetes.InfrastructureClusterConfig(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting infrastructure cluster config.")
	}
	opts := manager.Options{
		MetricsBindAddress:      "0",
		Namespace:               cfg().ProwJobNamespace,
		LeaderElection:          true,
		LeaderElectionNamespace: cfg().ProwJobNamespace,
		LeaderElectionID:        "prow-controller-manager-leader-lock",
	}
	mgr, err := manager.New(infrastructureClusterConfig, opts)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating manager")
	}

	buildManagers, err := o.kubernetes.BuildClusterManagers(o.dryRun,
		func(o *manager.Options) {
			o.Namespace = cfg().PodNamespace
		},
	)
	if err != nil {
		logrus.WithError(err).Error("Failed to construct build cluster managers. Is there a bad entry in the kubeconfig secret?")
	}

	for _, buildManager := range buildManagers {
		if err := mgr.Add(buildManager); err != nil {
			logrus.WithError(err).Fatal("Failed to add build cluster manager to main manager")
		}
	}

	// The watch apimachinery doesn't support restarts, so just exit the binary if a kubeconfig changes
	// to make the kubelet restart us.
	if err := o.kubernetes.AddKubeconfigChangeCallback(func() {
		logrus.Info("Kubeconfig changed, exiting to trigger a restart")
		interrupts.Terminate()
	}); err != nil {
		logrus.WithError(err).Fatal("Failed to register kubeconfig change callback")
	}

	enabledControllersSet := sets.NewString(o.enabledControllers.Strings()...)

	if enabledControllersSet.Has(plank.ControllerName) {
		if err := plank.Add(mgr, buildManagers, cfg, o.totURL, o.selector); err != nil {
			logrus.WithError(err).Fatal("Failed to add plank to manager")
		}
	}

	// Expose prometheus metrics
	metrics.ExposeMetrics("plank", cfg().PushGateway, o.instrumentationOptions.MetricsPort)
	if err := mgr.Start(interrupts.Context()); err != nil {
		logrus.WithError(err).Fatal("failed to start manager")
	}

	logrus.Info("Controller ended gracefully")
}
