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
	"encoding/json"
	"errors"
	"flag"
	"fmt"

	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/test-infra/prow/kube"

	"k8s.io/test-infra/prow/artifact-uploader"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pod-utils/options"
)

// newOptions returns an empty Options with no nil fields
func newOptions() *Options {
	return &Options{
		Options: gcsupload.NewOptions(),
	}
}

// Options holds info about parallelism, how to upload and cluster credentials.
type Options struct {
	// NumWorkers determines the number of workers that consume
	// the controller's work queue
	NumWorkers int `json:"num_workers"`
	// ProwJobNamespace is the namespace in the cluster that holds
	// ProwJob objects
	ProwJobNamespace string `json:"'prow_job_namespace'"`

	*gcsupload.Options

	clusterConfig *rest.Config
}

// Validate ensures that the set of options are
// self-consistent and valid
func (o *Options) Validate() error {
	if o.NumWorkers == 0 {
		return errors.New("number of workers cannot be zero")
	}

	if o.ProwJobNamespace == "" {
		return errors.New("namespace containing ProwJobs not configured")
	}

	return o.Options.Validate()
}

const (
	// JSONConfigEnvVar is the environment variable that
	// utilities expect to find a full JSON configuration
	// in when run.
	JSONConfigEnvVar = "ARTIFACTUPLOAD_OPTIONS"
)

// ConfigVar exposes the environment variable used
// to store serialized configuration
func (o *Options) ConfigVar() string {
	return JSONConfigEnvVar
}

// LoadConfig loads options from serialized config
func (o *Options) LoadConfig(config string) error {
	return json.Unmarshal([]byte(config), o)
}

// AddFlags binds flags to options
func (o *Options) AddFlags(flags *flag.FlagSet) {
	flags.IntVar(&o.NumWorkers, "num-workers", 25, "Number of threads to use for processing updates.")
	flags.StringVar(&o.ProwJobNamespace, "prow-job-ns", "", "Namespace containing ProwJobs.")
	o.Options.AddFlags(flags)
}

// Complete internalizes command line arguments
func (o *Options) Complete(args []string) {
	o.Options.Complete(args)
}

// loadClusterConfig loads connection configuration
// for the cluster we're deploying to. We prefer to
// use in-cluster configuration if possible, but will
// fall back to using default rules otherwise.
func loadClusterConfig() (*rest.Config, error) {
	clusterConfig, err := rest.InClusterConfig()
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

// Run uploads artifacts with the specified options forever.
//
// Sends a stop message to the artifact uploader when it is interrupted.
func (o *Options) Run() error {
	clusterConfig, err := loadClusterConfig()
	if err != nil {
		return fmt.Errorf("failed to load cluster config: %v", err)
	}

	client, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		return err
	}

	prowJobClient, err := kube.NewClientInCluster(o.ProwJobNamespace)
	if err != nil {
		return err
	}

	controller := artifact_uploader.NewController(client.CoreV1(), prowJobClient, o.Options)

	stop := make(chan struct{})
	defer close(stop)
	go controller.Run(o.NumWorkers, stop)

	// Wait forever
	select {}
}

func main() {
	o := newOptions()
	if err := options.Load(o); err != nil {
		logrus.Fatalf("Could not resolve options: %v", err)
	}

	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "artifact-uploader"}),
	)

	if err := o.Run(); err != nil {
		logrus.WithError(err).Fatal("Failed to run the GCS uploader controller")
	}
}
