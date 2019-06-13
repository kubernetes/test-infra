/*
Copyright 2019 The Kubernetes Authors.

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
	"os/signal"
	"syscall"
	"time"

	prowjobset "k8s.io/test-infra/prow/client/clientset/versioned"
	prowjobinfo "k8s.io/test-infra/prow/client/informers/externalversions"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"

	pipelineset "github.com/tektoncd/pipeline/pkg/client/clientset/versioned"
	pipelineinfo "github.com/tektoncd/pipeline/pkg/client/informers/externalversions"
	pipelineinfov1alpha1 "github.com/tektoncd/pipeline/pkg/client/informers/externalversions/pipeline/v1alpha1"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // support gcp users in .kube/config
)

type options struct {
	allContexts  bool
	buildCluster string
	config       string
	kubeconfig   string
	totURL       string
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
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("Parse flags: %v", err)
	}
	if o.kubeconfig != "" && o.buildCluster != "" {
		return errors.New("deprecated --builde-cluster may not be used with --kubeconfig")
	}
	if o.buildCluster != "" {
		// TODO(fejta): change to warn and add a term date after plank migration
		logrus.Infof("--build-custer is deprecated, please switch to --kubeconfig")
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

type pipelineConfig struct {
	client   pipelineset.Interface
	informer pipelineinfov1alpha1.PipelineRunInformer
}

// newPipelineConfig returns a client and informer capable of mutating and monitoring the specified config.
func newPipelineConfig(cfg rest.Config, stop chan struct{}) (*pipelineConfig, error) {
	bc, err := pipelineset.NewForConfig(&cfg)
	if err != nil {
		return nil, err
	}

	// Ensure the pipeline CRD is deployed
	// TODO(fejta): probably a better way to do this
	if _, err := bc.TektonV1alpha1().PipelineRuns("").List(metav1.ListOptions{Limit: 1}); err != nil {
		return nil, err
	}

	// Assume watches receive updates, but resync every 30m in case something wonky happens
	bif := pipelineinfo.NewSharedInformerFactory(bc, 30*time.Minute)
	bif.Tekton().V1alpha1().PipelineRuns().Lister()
	go bif.Start(stop)
	return &pipelineConfig{
		client:   bc,
		informer: bif.Tekton().V1alpha1().PipelineRuns(),
	}, nil
}

func main() {
	logrusutil.ComponentInit("pipeline")

	o := parseOptions()

	pjutil.ServePProf()

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
		logrus.WithError(err).Fatalf("Failed to create local kubernetes client")
	}
	pjc, err := prowjobset.NewForConfig(&local)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to create prowjob client")
	}
	pjif := prowjobinfo.NewSharedInformerFactory(pjc, 30*time.Minute)
	pjif.Prow().V1().ProwJobs().Lister()
	go pjif.Start(stop)

	pipelineConfigs := map[string]pipelineConfig{}
	for context, cfg := range configs {
		var bc *pipelineConfig
		bc, err = newPipelineConfig(cfg, stop)
		if apierrors.IsNotFound(err) {
			logrus.WithError(err).Warnf("Ignoring %s: knative pipeline CRD not deployed", context)
			continue
		}
		if err != nil {
			logrus.WithError(err).Fatalf("Failed to create %s pipeline client", context)
		}
		pipelineConfigs[context] = *bc
	}

	opts := controllerOptions{
		kc:              kc,
		pjc:             pjc,
		pji:             pjif.Prow().V1().ProwJobs(),
		pipelineConfigs: pipelineConfigs,
		totURL:          o.totURL,
		prowConfig:      configAgent.Config,
		rl:              kube.RateLimiter(controllerName),
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
