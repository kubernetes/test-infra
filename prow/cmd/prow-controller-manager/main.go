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
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	uberzap "go.uber.org/zap"
	"k8s.io/apimachinery/pkg/labels"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pjutil/pprof"
	"k8s.io/test-infra/prow/scheduler"
	ctrlruntimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"k8s.io/test-infra/pkg/flagutil"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/plank"

	_ "k8s.io/test-infra/prow/version"
)

var allControllers = sets.New(plank.ControllerName, scheduler.ControllerName)

type options struct {
	totURL string

	config             configflagutil.ConfigOptions
	selector           string
	enabledControllers prowflagutil.Strings

	dryRun                 bool
	kubernetes             prowflagutil.KubernetesOptions
	github                 prowflagutil.GitHubOptions // TODO(fejta): remove
	instrumentationOptions prowflagutil.InstrumentationOptions
	storage                prowflagutil.StorageClientOptions
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	o.enabledControllers = prowflagutil.NewStrings(plank.ControllerName)
	fs.StringVar(&o.totURL, "tot-url", "", "Tot URL")

	fs.StringVar(&o.selector, "label-selector", labels.Everything().String(), "Label selector to be applied in prowjobs. See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors for constructing a label selector.")
	fs.Var(&o.enabledControllers, "enable-controller", fmt.Sprintf("Controllers to enable. Can be passed multiple times. Defaults to controllers: %s", plank.ControllerName))

	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to GitHub.")
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github, &o.instrumentationOptions, &o.config, &o.storage} {
		group.AddFlags(fs)
	}

	fs.Parse(args)
	return o
}

func (o *options) Validate() error {
	o.github.AllowAnonymous = true

	var errs []error
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github, &o.instrumentationOptions, &o.config, &o.storage} {
		if err := group.Validate(o.dryRun); err != nil {
			errs = append(errs, err)
		}
	}

	for _, enabledController := range o.enabledControllers.Strings() {
		if !allControllers.Has(enabledController) {
			errs = append(errs, fmt.Errorf("unknown controller %s was configured via --enabled-controller", enabledController))
		}
	}

	if n := len(allControllers.Intersection(sets.New(o.enabledControllers.Strings()...))); n == 0 {
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

	health := pjutil.NewHealthOnPort(o.instrumentationOptions.HealthPort) // Start liveness endpoint
	pprof.Instrument(o.instrumentationOptions)

	configAgent, err := o.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := configAgent.Config
	o.kubernetes.SetDisabledClusters(sets.New(cfg().DisabledClusters...))

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

	// The watch apimachinery doesn't support restarts, so just exit the
	// binary if a build cluster can be connected later.
	callBack := func() {
		logrus.Info("Build cluster that failed to connect initially now worked, exiting to trigger a restart.")
		interrupts.Terminate()
	}

	buildClusterManagers, err := o.kubernetes.BuildClusterManagers(o.dryRun,
		plank.RequiredTestPodVerbs(),
		callBack,
		func(o *manager.Options) {
			o.Namespace = cfg().PodNamespace
		},
	)
	if err != nil {
		logrus.WithError(err).Error("Failed to construct build cluster managers. Please check that the kubeconfig secrets are correct, and that RBAC roles on the build cluster allow Prow's service account to list pods on it.")
	}

	for buildClusterName, buildClusterManager := range buildClusterManagers {
		if err := mgr.Add(buildClusterManager); err != nil {
			logrus.WithError(err).WithFields(logrus.Fields{
				"cluster": buildClusterName,
			}).Fatalf("Failed to add build cluster manager to main manager")
		}
	}

	opener, err := io.NewOpener(context.Background(), o.storage.GCSCredentialsFile, o.storage.S3CredentialsFile)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating opener")
	}

	// The watch apimachinery doesn't support restarts, so just exit the binary if a kubeconfig changes
	// to make the kubelet restart us.
	if err := o.kubernetes.AddKubeconfigChangeCallback(func() {
		logrus.Info("Kubeconfig changed, exiting to trigger a restart")
		interrupts.Terminate()
	}); err != nil {
		logrus.WithError(err).Fatal("Failed to register kubeconfig change callback")
	}

	enabledControllersSet := sets.New(o.enabledControllers.Strings()...)
	knownClusters, err := o.kubernetes.KnownClusters(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to resolve known clusters in kubeconfig.")
	}

	if enabledControllersSet.Has(plank.ControllerName) {
		if err := plank.Add(mgr, buildClusterManagers, knownClusters, cfg, opener, o.totURL, o.selector); err != nil {
			logrus.WithError(err).Fatal("Failed to add plank to manager")
		}
	}

	if enabledControllersSet.Has(scheduler.ControllerName) {
		if err := scheduler.Add(mgr, cfg, 1); err != nil {
			logrus.WithError(err).Fatal("Failed to add scheduler to manager")
		}
	}

	// Expose prometheus metrics
	metrics.ExposeMetrics("plank", cfg().PushGateway, o.instrumentationOptions.MetricsPort)
	// Serve readiness endpoint
	health.ServeReady()

	if err := mgr.Start(interrupts.Context()); err != nil {
		logrus.WithError(err).Fatal("failed to start manager")
	}

	logrus.Info("Controller ended gracefully")
}
