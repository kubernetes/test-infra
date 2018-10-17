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

	prowjobset "k8s.io/test-infra/prow/client/clientset/versioned"
	prowjobinfo "k8s.io/test-infra/prow/client/informers/externalversions"
	"k8s.io/test-infra/prow/logrusutil"

	buildset "github.com/knative/build/pkg/client/clientset/versioned"
	buildinfo "github.com/knative/build/pkg/client/informers/externalversions"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type options struct {
	masterURL  string
	kubeconfig string
	totURL     string

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
	flags.StringVar(&o.totURL, "tot-url", "", "Tot URL")
	flags.StringVar(&o.kubeconfig, "kubeconfig", "", "Path to kubeconfig. Only required if out of cluster")
	flags.StringVar(&o.masterURL, "master", "", "The address of the kubernetes API server. Overrides any value in kubeconfig. Only required if out of cluster")
	flags.StringVar(&o.cert, "tls-cert-file", "", "Path to x509 certificate for HTTPS")
	flags.StringVar(&o.privateKey, "tls-private-key-file", "", "Path to matching x509 private key.")
	flags.Parse(args)
	if (len(o.cert) == 0) != (len(o.privateKey) == 0) {
		return errors.New("Both --tls-cert-file and --tls-private-key-file are required for HTTPS")
	}
	return nil
}

func stopper() chan struct{} {
	stop := make(chan struct{})
	c := make(chan os.Signal, 2)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		close(stop)
		<-c
		os.Exit(1)
	}()
	return stop
}

func main() {
	o := parseOptions()
	logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "build"})
	stop := stopper()

	cfg, err := clientcmd.BuildConfigFromFlags(o.masterURL, o.kubeconfig)
	if err != nil {
		logrus.Fatalf("Error building kubeconfig: %v", err)
	}

	kc, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		logrus.Fatalf("Error building kubernetes client: %v", err)
	}

	pjc, err := prowjobset.NewForConfig(cfg)
	if err != nil {
		logrus.Fatalf("Error building prowjob client: %v", err)
	}

	bc, err := buildset.NewForConfig(cfg)
	if err != nil {
		logrus.Fatalf("Error building build client: %v", err)
	}

	// Assume watches receive updates, but resync every 30m in case something wonky happens
	pjif := prowjobinfo.NewSharedInformerFactory(pjc, time.Minute)
	bif := buildinfo.NewSharedInformerFactory(bc, time.Minute)

	controller := newController(kc, pjc, bc, pjif.Prow().V1().ProwJobs(), bif.Build().V1alpha1().Builds(), o.totURL)

	go pjif.Start(stop)
	go bif.Start(stop)

	if len(o.cert) > 0 {
		go runServer(o.cert, o.privateKey)
	}

	if err = controller.Run(2, stop); err != nil {
		logrus.Fatalf("Error running controller: %v", err)
	}
}
