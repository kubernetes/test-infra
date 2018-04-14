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
	"flag"
	"os"
	"time"

	"github.com/golang/glog"
	"k8s.io/test-infra/dind/pkg/cluster-up/cluster"
	"k8s.io/test-infra/dind/pkg/cluster-up/options"
	"k8s.io/test-infra/dind/pkg/util"
)

const (
	masterConfigDir = "/var/kubernetes"
	kubecfg         = "/var/kubernetes/admin.conf"
	timeout         = time.Duration(5) * time.Minute
)

func main() {
	// Discard the error, because flag.CommandLine is already set to ExitOnError.
	o, _ := options.New(flag.CommandLine, os.Args[1:])
	defer glog.Flush()

	// Dynamically lookup any arguments that weren't provided.
	if o.Version == "" {
		v, err := util.DockerImageVersion()
		if err != nil {
			glog.Fatalf("Failed to load docker image version: %v", err)
		}
		o.Version = v
	}

	if o.ProxyAddr == "" {
		p, err := util.NonLocalUnicastAddr("eth0")
		if err != nil {
			glog.Fatalf("Failed to determine external proxy address.")
		}
		o.ProxyAddr = p
	}
	glog.Infof("External proxy for alt SAN is %s", o.ProxyAddr)

	// Create a cluster, and wait for it to come up.
	c, err := cluster.New(o.NumNodes, o.ProxyAddr, o.DinDNodeImage, "/var/kubernetes", "testnet", "/dind-node-bundle.tar", o.Version, timeout)

	if err != nil {
		glog.Fatalf("Failed to create a dind cluster object: %v", err)
	}
	glog.Infof("Starting a cluster.")
	if err := c.Up(o.SideloadImage); err != nil {
		glog.Fatalf("Failed to wait for cluster to come up: %v", err)
	}
	glog.Infof("Cluster is initialized.")
}
