/*
Copyright 2016 The Kubernetes Authors.

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
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	ctrlruntimelog "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/signals"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/sinker"
)

type options struct {
	runOnce       bool
	configPath    string
	jobConfigPath string
	dryRun        flagutil.Bool
	kubernetes    flagutil.ExperimentalKubernetesOptions
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	o := options{}
	fs.BoolVar(&o.runOnce, "run-once", false, "If true, run only once then quit.")
	fs.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")

	// TODO(fejta): switch dryRun to be a bool, defaulting to true after March 15, 2019.
	fs.Var(&o.dryRun, "dry-run", "Whether or not to make mutating API calls to Kubernetes.")

	o.kubernetes.AddFlags(fs)
	fs.Parse(args)
	o.configPath = config.ConfigPath(o.configPath)
	return o
}

func (o *options) Validate() error {
	if err := o.kubernetes.Validate(o.dryRun.Value); err != nil {
		return err
	}

	if o.configPath == "" {
		return errors.New("--config-path is required")
	}

	return nil
}

func main() {
	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	pjutil.ServePProf()

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "sinker"}),
	)

	if !o.dryRun.Explicit {
		logrus.Warning("Sinker requires --dry-run=false to function correctly in production.")
		logrus.Warning("--dry-run will soon default to true. Set --dry-run=false by March 15.")
	}

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	cfg := configAgent.Config

	pushGateway := cfg().PushGateway
	metrics.ExposeMetrics("sinker", pushGateway.Endpoint, pushGateway.Interval.Duration)

	controlClusterCfg, err := o.kubernetes.ControlClusterConfig(o.dryRun.Value)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting controlcluster config.")
	}

	buildClusterClients, err := o.kubernetes.BuildClusterClients(cfg().PodNamespace, o.dryRun.Value)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating build cluster clients.")
	}

	var podClients []corev1.PodInterface
	for _, client := range buildClusterClients {
		// sinker doesn't care about build cluster aliases
		podClients = append(podClients, client)
	}

	// Enabling debug logging has the unfortunate side-effect of making the log
	// unstructured
	// https://github.com/kubernetes-sigs/controller-runtime/issues/442
	ctrlruntimelog.SetLogger(ctrlruntimelog.ZapLogger(cfg().LogLevel == "debug"))

	mgr, err := manager.New(controlClusterCfg, manager.Options{
		LeaderElection:          true,
		LeaderElectionNamespace: metav1.NamespaceSystem,
		LeaderElectionID:        "sinker-leader-election",
		Namespace:               cfg().ProwJobNamespace,
	})
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create mgr")
	}

	if err := prowapi.AddToScheme(mgr.GetScheme()); err != nil {
		logrus.WithError(err).Fatal("Error adding prow types to scheme")
	}

	if err := sinker.Add(mgr, logrus.NewEntry(logrus.StandardLogger()), podClients, cfg, cfg().Sinker.ResyncPeriod.Duration, o.runOnce, o.dryRun.Value); err != nil {
		logrus.WithError(err).Fatal("Error adding sinker to mgr")
	}

	stopCh := signals.SetupSignalHandler()
	if err := mgr.Start(stopCh); err != nil {
		logrus.WithError(err).Fatal("Failed to start mgr")
	}

}
