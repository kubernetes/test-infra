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
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/time/rate"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/workqueue"

	prowjobclientset "k8s.io/test-infra/prow/client/clientset/versioned"
	prowjobinformer "k8s.io/test-infra/prow/client/informers/externalversions"

	"k8s.io/test-infra/prow/crier"
	gerritclient "k8s.io/test-infra/prow/gerrit/client"
	gerritreporter "k8s.io/test-infra/prow/gerrit/reporter"
	"k8s.io/test-infra/prow/logrusutil"
	pubsubreporter "k8s.io/test-infra/prow/pubsub/reporter"
)

const (
	resync = 0 * time.Minute
)

type options struct {
	masterURL      string
	kubeConfig     string
	cookiefilePath string
	gerritProjects gerritclient.ProjectsFlag

	gerritWorkers int
	pubsubWorkers int
	// githubWorkers int
}

func (o *options) Validate() error {
	if o.gerritWorkers > 1 {
		// TODO(krzyzacy): try to see how to handle racy better for gerrit aggregate report.
		logrus.Warn("gerrit reporter only supports one worker")
		o.gerritWorkers = 1
	}

	if o.gerritWorkers+o.pubsubWorkers <= 0 {
		return errors.New("crier need to have at least one report worker to start")
	}

	if o.gerritWorkers > 0 {
		if len(o.gerritProjects) == 0 {
			return errors.New("--gerrit-projects must be set")
		}

		if o.cookiefilePath == "" {
			logrus.Info("--cookiefile is not set, using anonymous authentication")
		}
	}

	return nil
}

func gatherOptions() options {
	o := options{
		gerritProjects: gerritclient.ProjectsFlag{},
	}

	flag.StringVar(&o.masterURL, "masterurl", "", "URL to k8s master")
	flag.StringVar(&o.kubeConfig, "kubeconfig", "", "Cluster config for the cluster you want to connect to")
	flag.StringVar(&o.cookiefilePath, "cookiefile", "", "Path to git http.cookiefile, leave empty for anonymous")
	flag.Var(&o.gerritProjects, "gerrit-projects", "Set of gerrit repos to monitor on a host example: --gerrit-host=https://android.googlesource.com=platform/build,toolchain/llvm, repeat flag for each host")
	flag.IntVar(&o.gerritWorkers, "gerrit-workers", 0, "Number of gerrit report workers (0 means disabled)")
	flag.IntVar(&o.pubsubWorkers, "pubsub-workers", 0, "Number of pubsub report workers (0 means disabled)")
	flag.Parse()
	return o
}

// TODO(krzyzacy): copy & paste, refactor this
// loadClusterConfig loads connection configuration
// for the cluster we're deploying to. We prefer to
// use in-cluster configuration if possible, but will
// fall back to using default rules otherwise.
func loadClusterConfig(masterURL, kubeConfig string) (*rest.Config, error) {
	clusterConfig, err := clientcmd.BuildConfigFromFlags(masterURL, kubeConfig)
	if err == nil {
		return clusterConfig, nil
	}

	credentials, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return nil, fmt.Errorf("could not load credentials from config: %v", err)
	}

	clusterConfig, err = clientcmd.NewDefaultClientConfig(*credentials, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("could not load client configuration: %v", err)
	}
	return clusterConfig, nil
}

// TODO(krzyzacy): move this to kube???
// getKubernetesClient retrieves the Kubernetes cluster
// client from within the cluster
func getKubernetesClient(masterURL, kubeConfig string) (kubernetes.Interface, prowjobclientset.Interface) {
	config, err := loadClusterConfig(masterURL, kubeConfig)
	if err != nil {
		logrus.Fatalf("failed to load cluster config: %v", err)
	}

	// generate the client based off of the config
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		logrus.Fatalf("getClusterConfig: %v", err)
	}

	prowjobClient, err := prowjobclientset.NewForConfig(config)
	if err != nil {
		logrus.Fatalf("getClusterConfig: %v", err)
	}

	logrus.Info("Successfully constructed k8s client")
	return client, prowjobClient
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "crier"}),
	)

	_, prowjobClient := getKubernetesClient(o.masterURL, o.kubeConfig)

	prowjobInformerFactory := prowjobinformer.NewSharedInformerFactory(prowjobClient, resync)

	queue := workqueue.NewRateLimitingQueue(
		workqueue.NewMaxOfRateLimiter(
			workqueue.NewItemExponentialFailureRateLimiter(1*time.Second, 60*time.Second),
			// 10 qps, 100 bucket size.  This is only for retry speed and its only the overall factor (not per item)
			&workqueue.BucketRateLimiter{Limiter: rate.NewLimiter(rate.Limit(10), 100)},
		))

	controllers := []*crier.Controller{}

	// track all worker status before shutdown
	wg := &sync.WaitGroup{}

	if o.gerritWorkers > 0 {
		informer := prowjobInformerFactory.Prow().V1().ProwJobs()
		gerritReporter, err := gerritreporter.NewReporter(o.cookiefilePath, o.gerritProjects, informer.Lister())
		if err != nil {
			logrus.Fatalf("Fail to start gerrit reporter: %v", err)
		}
		controllers = append(controllers, crier.NewController(prowjobClient, queue, informer, gerritReporter, o.gerritWorkers, wg))
	}

	if o.pubsubWorkers > 0 {
		controllers = append(controllers, crier.NewController(prowjobClient, queue, prowjobInformerFactory.Prow().V1().ProwJobs(), pubsubreporter.NewReporter(), o.pubsubWorkers, wg))
	}

	if len(controllers) == 0 {
		logrus.Fatalf("should have at least one controller to start crier.")
	}

	stopCh := make(chan struct{})
	defer close(stopCh)

	// run the controller loop to process items
	prowjobInformerFactory.Start(stopCh)
	for _, controller := range controllers {
		go controller.Run(stopCh)
	}

	sigTerm := make(chan os.Signal, 1)
	signal.Notify(sigTerm, syscall.SIGTERM)
	signal.Notify(sigTerm, syscall.SIGINT)

	<-sigTerm
	logrus.Info("Crier received a termination signal and is shutting down...")
	for range controllers {
		stopCh <- struct{}{}
	}

	// waiting for all crier worker to finish
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		logrus.Info("All worker finished, exiting crier")
	case <-time.After(10 * time.Second):
		logrus.Info("timed out waiting for all worker to finish")
	}
}
