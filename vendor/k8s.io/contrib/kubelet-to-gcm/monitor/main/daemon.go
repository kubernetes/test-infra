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
	"time"

	log "github.com/golang/glog"
	"github.com/spf13/pflag"
	"golang.org/x/net/context"
	"golang.org/x/oauth2/google"

	v3 "google.golang.org/api/monitoring/v3"
	"k8s.io/contrib/kubelet-to-gcm/monitor"
	"k8s.io/contrib/kubelet-to-gcm/monitor/config"
	"k8s.io/contrib/kubelet-to-gcm/monitor/controller"
	"k8s.io/contrib/kubelet-to-gcm/monitor/kubelet"
)

const (
	scope = "https://www.googleapis.com/auth/monitoring.write"
	//testPath = "https://test-monitoring.sandbox.googleapis.com"
)

var (
	// Flags to identify the Kubelet.
	zone        = pflag.String("zone", "use-gce", "The zone where this kubelet lives.")
	project     = pflag.String("project", "use-gce", "The project where this kubelet's host lives.")
	cluster     = pflag.String("cluster", "use-gce", "The cluster where this kubelet holds membership.")
	kubeletHost = pflag.String("kubelet-host", "use-gce", "The kubelet's host name.")
	kubeletPort = pflag.Uint("kubelet-port", 10255, "The kubelet's port.")
	ctrlPort    = pflag.Uint("controller-manager-port", 10252, "The kube-controller's port.")
	// Flags to control runtime behavior.
	res         = pflag.Uint("resolution", 10, "The time, in seconds, to poll the Kubelet.")
	gcmEndpoint = pflag.String("gcm-endpoint", "", "The GCM endpoint to hit. Defaults to the default endpoint.")
)

func main() {
	// First log our starting config, and then set up.
	flag.Set("logtostderr", "true") // This spoofs glog into teeing logs to stderr.
	defer log.Flush()
	pflag.Parse()
	log.Infof("Invoked by %v", os.Args)

	resolution := time.Second * time.Duration(*res)

	// Initialize the configuration.
	kubeletCfg, ctrlCfg, err := config.NewConfigs(*zone, *project, *cluster, *kubeletHost, *kubeletPort, *ctrlPort, resolution)
	if err != nil {
		log.Fatalf("Failed to initialize configuration: %v", err)
	}

	// Create objects for kubelet monitoring.
	kubeletSrc, err := kubelet.NewSource(kubeletCfg)
	if err != nil {
		log.Fatalf("Failed to create a kubelet source with config %v: %v", kubeletCfg, err)
	}
	log.Infof("The kubelet source is initialized with config %v.", kubeletCfg)

	// Create objects for kube-controller monitoring.
	ctrlSrc, err := controller.NewSource(ctrlCfg)
	if err != nil {
		log.Fatalf("Failed to create a kube-controller source with config %v: %v", ctrlCfg, err)
	}
	log.Infof("The kube-controller source is initialized with config %v.", ctrlCfg)

	// Create a GCM client.
	client, err := google.DefaultClient(context.Background(), scope)
	if err != nil {
		log.Fatalf("Failed to create a client with default context and scope %s, err: %v", scope, err)
	}
	service, err := v3.New(client)
	if err != nil {
		log.Fatalf("Failed to create a GCM v3 API service object: %v", err)
	}
	// Determine the GCE endpoint.
	if *gcmEndpoint != "" {
		service.BasePath = *gcmEndpoint
	}
	log.Infof("Using GCM endpoint %q", service.BasePath)

	for {
		go monitor.Once(kubeletSrc, service)
		go monitor.Once(ctrlSrc, service)
		time.Sleep(resolution)
	}
}
