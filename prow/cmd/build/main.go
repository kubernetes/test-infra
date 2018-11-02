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
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	prowjobset "k8s.io/test-infra/prow/client/clientset/versioned"
	prowjobinfo "k8s.io/test-infra/prow/client/informers/externalversions"
	"k8s.io/test-infra/prow/logrusutil"

	buildset "github.com/knative/build/pkg/client/clientset/versioned"
	buildinfo "github.com/knative/build/pkg/client/informers/externalversions"
	buildinfov1alpha1 "github.com/knative/build/pkg/client/informers/externalversions/build/v1alpha1"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type options struct {
	kubeconfig string
	totURL     string

	// Create these values by following:
	//   https://github.com/kelseyhightower/grafeas-tutorial/blob/master/pki/gen-certs.sh
	cert        string
	privateKey  string
	allContexts bool
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
	flags.StringVar(&o.cert, "tls-cert-file", "", "Path to x509 certificate for HTTPS")
	flags.StringVar(&o.privateKey, "tls-private-key-file", "", "Path to matching x509 private key.")
	flags.Parse(args)
	if (len(o.cert) == 0) != (len(o.privateKey) == 0) {
		return errors.New("Both --tls-cert-file and --tls-private-key-file are required for HTTPS")
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

// contextConfigs returns a context => config mapping as well as the default context.
//
// Returns an error if kubeconfig is specified and invalid
// Returns an error if no contexts are found.
func contextConfigs(kubeconfig string) (map[string]rest.Config, string, error) {
	configs := map[string]rest.Config{}
	var defCtx *string
	if localCfg, err := rest.InClusterConfig(); err != nil {
		logrus.Warnf("Failed to create in-cluster config: %v", err)
	} else {
		defCtx = new(string)
		configs[*defCtx] = *localCfg
	}

	var loader clientcmd.ClientConfigLoader
	if kubeconfig != "" {
		loader = &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfig}
	} else {
		loader = clientcmd.NewDefaultClientConfigLoadingRules()
	}

	cfg, err := loader.Load()
	switch {
	case err != nil && kubeconfig != "":
		return nil, "", fmt.Errorf("load %s: %v", kubeconfig, err)
	case err != nil:
		logrus.Warnf("failed to load any kubecfg files: %v", err)
	default:
		if defCtx == nil && cfg.CurrentContext != "" {
			defCtx = &cfg.CurrentContext
		}
		for context := range cfg.Contexts {
			contextCfg, err := clientcmd.NewNonInteractiveClientConfig(*cfg, context, nil, loader).ClientConfig()
			if err != nil {
				return nil, "", fmt.Errorf("create %s client: %v", context, err)
			}
			configs[context] = *contextCfg
		}
	}

	if len(configs) == 0 {
		return nil, "", errors.New("no clients found")
	}
	return configs, *defCtx, nil
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
	// Assume watches receive updates, but resync every 30m in case something wonky happens
	bif := buildinfo.NewSharedInformerFactory(bc, time.Minute)
	go bif.Start(stop)
	return &buildConfig{
		client:   bc,
		informer: bif.Build().V1alpha1().Builds(),
	}, nil
}

func main() {
	o := parseOptions()
	logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "build"})

	configs, defaultContext, err := contextConfigs(o.kubeconfig)
	if err != nil {
		logrus.Fatalf("Error building configs: %v", err)
	}

	if !o.allContexts { // Just the default context please
		configs = map[string]rest.Config{defaultContext: configs[defaultContext]}
	}
	defaultConfig := configs[defaultContext]

	stop := stopper()

	kc, err := kubernetes.NewForConfig(&defaultConfig)
	if err != nil {
		logrus.Fatalf("Failed to create kubernetes client: %v", err)
	}
	pjc, err := prowjobset.NewForConfig(&defaultConfig)
	if err != nil {
		logrus.Fatalf("Failed to create prowjob client: %v", err)
	}
	pjif := prowjobinfo.NewSharedInformerFactory(pjc, time.Minute)
	go pjif.Start(stop)

	buildConfigs := map[string]buildConfig{}
	for context, cfg := range configs {
		var bc *buildConfig
		bc, err = newBuildConfig(cfg, stop)
		if err != nil {
			logrus.Fatalf("Failed to create %s build client: %v", context, err)
		}
		buildConfigs[context] = *bc
	}

	// TODO(fejta): move to its own binary
	if len(o.cert) > 0 {
		go runServer(o.cert, o.privateKey)
	}

	controller := newController(kc, pjc, pjif.Prow().V1().ProwJobs(), buildConfigs, o.totURL)
	if err := controller.Run(2, stop); err != nil {
		logrus.Fatalf("Error running controller: %v", err)
	}
	logrus.Info("Finished")
}
