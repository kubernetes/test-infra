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
	"errors"
	"flag"
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

	buildset "github.com/knative/build/pkg/client/clientset/versioned"
	buildinfo "github.com/knative/build/pkg/client/informers/externalversions"
	buildinfov1alpha1 "github.com/knative/build/pkg/client/informers/externalversions/build/v1alpha1"
	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // support gcp users in .kube/config
	"k8s.io/client-go/rest"
)

type options struct {
	allContexts  bool
	buildCluster string
	config       string
	kubeconfig   string
	totURL       string

	// Create these values by following:
	//   https://github.com/kelseyhightower/grafeas-tutorial/blob/master/pki/gen-certs.sh
	cert       string
	privateKey string
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
	flags.StringVar(&o.cert, "tls-cert-file", "", "Path to x509 certificate for HTTPS")
	flags.StringVar(&o.privateKey, "tls-private-key-file", "", "Path to matching x509 private key.")
	flags.Parse(args)
	if (len(o.cert) == 0) != (len(o.privateKey) == 0) {
		return errors.New("Both --tls-cert-file and --tls-private-key-file are required for HTTPS")
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
	client   buildset.Interface
	informer buildinfov1alpha1.BuildInformer
}

// newBuildConfig returns a client and informer capable of mutating and monitoring the specified config.
func newBuildConfig(cfg rest.Config, stop chan struct{}) (*buildConfig, error) {
	bc, err := buildset.NewForConfig(&cfg)
	if err != nil {
		return nil, err
	}

	// Ensure the knative-build CRD is deployed
	// TODO(fejta): probably a better way to do this
	_, err = bc.Build().Builds("").List(metav1.ListOptions{Limit: 1})
	if err != nil {
		return nil, err
	}
	// Assume watches receive updates, but resync every 30m in case something wonky happens
	bif := buildinfo.NewSharedInformerFactory(bc, 30*time.Minute)
	bif.Build().V1alpha1().Builds().Lister()
	go bif.Start(stop)
	return &buildConfig{
		client:   bc,
		informer: bif.Build().V1alpha1().Builds(),
	}, nil
}

func main() {
	o := parseOptions()
	logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "build"})

	configAgent := &config.Agent{}
	if o.config != "" {
		const ignoreJobConfig = ""
		if err := configAgent.Start(o.config, ignoreJobConfig); err != nil {
			logrus.WithError(err).Fatal("failed to load prow config")
		}
	}

	configs, defaultContext, err := kube.LoadClusterConfigs(o.kubeconfig, o.buildCluster)
	if err != nil {
		logrus.WithError(err).Fatal("Error building client configs")
	}

	if !o.allContexts { // Just the default context please
		logrus.Warnf("Truncating to a single cluster: %s", defaultContext)
		configs = map[string]rest.Config{defaultContext: configs[defaultContext]}
	}
	defaultConfig := configs[defaultContext]

	stop := stopper()

	kc, err := kubernetes.NewForConfig(&defaultConfig)
	if err != nil {
		logrus.WithError(err).Fatalf("Failed to create %s kubernetes client", defaultContext)
	}
	pjc, err := prowjobset.NewForConfig(&defaultConfig)
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

	// TODO(fejta): move to its own binary
	if len(o.cert) > 0 {
		go runServer(o.cert, o.privateKey)
	}

	controller := newController(kc, pjc, pjif.Prow().V1().ProwJobs(), buildConfigs, o.totURL, configAgent.Config, kube.RateLimiter(controllerName))
	if err := controller.Run(2, stop); err != nil {
		logrus.WithError(err).Fatal("Error running controller")
	}
	logrus.Info("Finished")
}
