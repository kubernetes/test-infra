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
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	prowjobset "k8s.io/test-infra/prow/client/clientset/versioned"
	prowjobinfo "k8s.io/test-infra/prow/client/informers/externalversions"
	"k8s.io/test-infra/prow/config"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	pipelineset "k8s.io/test-infra/prow/pipeline/clientset/versioned"
	pipelineinfo "k8s.io/test-infra/prow/pipeline/informers/externalversions"
	pipelineinfov1alpha1 "k8s.io/test-infra/prow/pipeline/informers/externalversions/pipeline/v1alpha1"
	"k8s.io/test-infra/prow/pjutil"

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // support gcp users in .kube/config
)

type options struct {
	allContexts            bool
	buildCluster           string
	configPath             string
	kubeconfig             string
	totURL                 string
	instrumentationOptions prowflagutil.InstrumentationOptions
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
	flags.StringVar(&o.configPath, "config", "", "Path to prow config.yaml")
	o.instrumentationOptions.AddFlags(flags)
	if err := flags.Parse(args); err != nil {
		return fmt.Errorf("Parse flags: %v", err)
	}
	if o.configPath == "" {
		return errors.New("--config is mandatory, set --config to prow config.yaml file")
	}
	return nil
}

type pipelineConfig struct {
	client   pipelineset.Interface
	informer pipelineinfov1alpha1.PipelineRunInformer
}

// newPipelineConfig returns a client and informer capable of mutating and monitoring the specified config.
func newPipelineConfig(cfg rest.Config, stop <-chan struct{}) (*pipelineConfig, error) {
	bc, err := pipelineset.NewForConfig(&cfg)
	if err != nil {
		return nil, err
	}

	// Ensure the pipeline CRD is deployed
	// TODO(fejta): probably a better way to do this
	if _, err := bc.TektonV1alpha1().PipelineRuns("").List(context.TODO(), metav1.ListOptions{Limit: 1}); err != nil {
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
	logrusutil.ComponentInit()

	o := parseOptions()

	defer interrupts.WaitForGracefulShutdown()

	pjutil.ServePProf(o.instrumentationOptions.PProfPort)

	configAgent := &config.Agent{}
	const ignoreJobConfig = ""
	if err := configAgent.Start(o.configPath, ignoreJobConfig); err != nil {
		logrus.WithError(err).Fatal("failed to load prow config")
	}

	configs, err := kube.LoadClusterConfigs(o.kubeconfig, "")
	if err != nil {
		logrus.WithError(err).Fatal("Error building client configs")
	}

	local := configs[kube.InClusterContext]
	if !o.allContexts {
		logrus.Warn("Truncating to default context")
		configs = map[string]rest.Config{
			kube.DefaultClusterAlias: configs[kube.DefaultClusterAlias],
		}
	} else {
		// the InClusterContext is always mapped to DefaultClusterAlias in the controller, so there is no need to watch for this config.
		delete(configs, kube.InClusterContext)
	}

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
	go pjif.Start(interrupts.Context().Done())

	pipelineConfigs := map[string]pipelineConfig{}
	for context, cfg := range configs {
		var bc *pipelineConfig
		bc, err = newPipelineConfig(cfg, interrupts.Context().Done())
		if apierrors.IsNotFound(err) {
			logrus.WithError(err).Infof("Ignoring cluster context %s: tekton pipeline CRD not deployed", context)
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

	if err := controller.Run(2, interrupts.Context().Done()); err != nil {
		logrus.WithError(err).Fatal("Error running controller")
	}
	logrus.Info("Finished")
}
