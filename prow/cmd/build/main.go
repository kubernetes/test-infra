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

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	prowjobset "k8s.io/test-infra/prow/client/clientset/versioned"
	prowjobinfo "k8s.io/test-infra/prow/client/informers/externalversions"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"

	buildv1alpha1 "github.com/knative/build/pkg/apis/build/v1alpha1"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // support gcp users in .kube/config
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

type options struct {
	allContexts  bool
	buildCluster string
	config       string
	kubeconfig   string
	totURL       string

	// This is a termporary flag which gates the usage of plank.allow_cancellations config value
	// for build aborter.
	// TODO remove this flag and use directly the config flag.
	useAllowCancellations bool
}

func parseOptions() options {
	var o options
	if err := o.parse(flag.CommandLine, os.Args[1:]); err != nil {
		logrus.Fatalf("Invalid flags: %v", err)
	}
	return o
}

func (o *options) parse(flags *flag.FlagSet, args []string) error {
	flags.BoolVar(&o.allContexts, "all-contexts", false, "Monitor all cluster contexts, not just default")
	flags.StringVar(&o.totURL, "tot-url", "", "Tot URL")
	flags.StringVar(&o.kubeconfig, "kubeconfig", "", "Path to kubeconfig. Only required if out of cluster")
	flags.StringVar(&o.config, "config", "", "Path to prow config.yaml")
	flags.StringVar(&o.buildCluster, "build-cluster", "", "Path to file containing a YAML-marshalled kube.Cluster object. If empty, uses the local cluster.")
	flags.BoolVar(&o.useAllowCancellations, "use-allow-cancellations", false, "Gates the usage of plank.allow_cancellations config flag for build aborter")
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("parse flags: %v", err)
	}
	if o.kubeconfig != "" && o.buildCluster != "" {
		return errors.New("deprecated --build-cluster may not be used with --kubeconfig")
	}
	if o.buildCluster != "" {
		// TODO(fejta): change to warn and add a term date after plank migration
		logrus.Infof("--build-cluster is deprecated, please switch to --kubeconfig")
	}
	return nil
}

// stopper returns a channel that remains open until an interrupt is received.
func stopper() chan struct{} {
	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		logrus.Warn("Interrupt received, attempting clean shutdown...")
		close(stop)
		<-c
		logrus.Error("Second interrupt received, force exiting...")
		os.Exit(1)
	}()
	return stop
}

type buildConfig struct {
	client ctrlruntimeclient.Client
	// Only use the informer to add EventHandlers, for getting
	// objects use the client instead, its Reader interface is
	// backed by the cache
	informer cache.Informer
}

// newBuildConfig returns a client and informer capable of mutating and monitoring the specified config.
func newBuildConfig(cfg rest.Config, stop chan struct{}) (*buildConfig, error) {
	// Assume watches receive updates, but resync every 30m in case something wonky happens
	resyncInterval := 30 * time.Minute
	// We construct a manager because it has a client whose Reader interface is backed by its cache, which
	// is really nice to use, but the corresponding code is not exported.
	mgr, err := manager.New(&cfg, manager.Options{SyncPeriod: &resyncInterval})
	if err != nil {
		return nil, err
	}

	// Ensure the knative-build CRD is deployed
	// TODO(fejta): probably a better way to do this
	buildList := &buildv1alpha1.BuildList{}
	opts := &ctrlruntimeclient.ListOptions{Raw: &metav1.ListOptions{Limit: 1}}
	if err := mgr.GetClient().List(context.TODO(), buildList, ctrlruntimeclient.UseListOptions(opts)); err != nil {
		return nil, err
	}
	cache := mgr.GetCache()
	informer, err := cache.GetInformer(&buildv1alpha1.Build{})
	if err != nil {
		return nil, fmt.Errorf("failed to get cache for buildv1alpha1.Build: %v", err)
	}
	go cache.Start(stop)
	return &buildConfig{
		client:   mgr.GetClient(),
		informer: informer,
	}, nil
}

func main() {
	logrusutil.ComponentInit("build")

	o := parseOptions()

	pjutil.ServePProf()

	if err := buildv1alpha1.AddToScheme(scheme.Scheme); err != nil {
		logrus.WithError(err).Fatal("failed to add buildv1alpha1 to scheme")
	}

	configAgent := &config.Agent{}
	if o.config != "" {
		const ignoreJobConfig = ""
		if err := configAgent.Start(o.config, ignoreJobConfig); err != nil {
			logrus.WithError(err).Fatal("failed to load prow config")
		}
	}

	configs, err := kube.LoadClusterConfigs(o.kubeconfig, o.buildCluster)
	if err != nil {
		logrus.WithError(err).Fatal("Error building client configs")
	}

	local := configs[kube.InClusterContext]
	if !o.allContexts {
		logrus.Warn("Truncating to default context")
		configs = map[string]rest.Config{
			kube.DefaultClusterAlias: configs[kube.DefaultClusterAlias],
		}
	}

	stop := stopper()

	kc, err := kubernetes.NewForConfig(&local)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create local kubernetes client")
	}
	pjc, err := prowjobset.NewForConfig(&local)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create prowjob client")
	}
	pjif := prowjobinfo.NewSharedInformerFactory(pjc, 30*time.Minute)
	pjif.Prow().V1().ProwJobs().Lister()
	go pjif.Start(stop)

	buildConfigs := map[string]buildConfig{}
	for context, cfg := range configs {
		var bc *buildConfig
		bc, err = newBuildConfig(cfg, stop)
		if apierrors.IsNotFound(err) {
			logrus.WithError(err).Warnf("Ignoring %s: knative build CRD not deployed", context)
			continue
		}
		if err != nil {
			logrus.WithError(err).Fatalf("Failed to create %s build client", context)
		}
		buildConfigs[context] = *bc
	}

	opts := controllerOptions{
		kc:                    kc,
		pjc:                   pjc,
		pji:                   pjif.Prow().V1().ProwJobs(),
		buildConfigs:          buildConfigs,
		totURL:                o.totURL,
		prowConfig:            configAgent.Config,
		rl:                    kube.RateLimiter(controllerName),
		useAllowCancellations: o.useAllowCancellations,
	}
	controller, err := newController(opts)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating controller")
	}
	if err := controller.Run(2, stop); err != nil {
		logrus.WithError(err).Fatal("Error running controller")
	}
	logrus.Info("Finished")
}
