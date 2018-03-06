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
	"time"

	"github.com/golang/glog"
	"k8s.io/test-infra/dind/pkg/cluster"
	"k8s.io/test-infra/dind/pkg/util"
)

const (
	masterConfigDir = "/var/kubernetes"
	kubecfg         = "/var/kubernetes/admin.conf"
	timeout         = time.Duration(5) * time.Minute
)

var (
	loadImage = true
	nodeImage = "k8s.gcr.io/dind-node-amd64"
	proxyAddr = ""
	version   = ""
	numNodes  = 4
)

func init() {
	flag.BoolVar(&loadImage, "load-image", true, "If the image needs to be loaded from the file-system.")
	flag.StringVar(&nodeImage, "node-image", nodeImage, "The node image to use.")
	flag.StringVar(&proxyAddr, "proxy-addr", "", "The externally facing address for kubeadm to add to SAN.")
	flag.StringVar(&version, "k8s-version", version, "The kubernetes version to spin up.")
	flag.IntVar(&numNodes, "num-nodes", 4, "The number of nodes to make, including the master if applicable.")
}

func main() {
	flag.Parse()
	defer glog.Flush()

	// Dynamically lookup any arguments that weren't provided.
	if version == "" {
		v, err := util.DockerImageVersion()
		if err != nil {
			glog.Fatalf("Failed to load docker image version: %v", err)
		}
		version = v
	}

	if proxyAddr == "" {
		p, err := util.NonLocalUnicastAddr("eth0")
		if err != nil {
			glog.Fatalf("Failed to determine external proxy address.")
		}
		proxyAddr = p
	}
	glog.Infof("External proxy for alt SAN is %s", proxyAddr)

	// Create a cluster, and wait for it to come up.
	c, err := cluster.New(numNodes, proxyAddr, nodeImage, "/var/kubernetes", "testnet", "/dind-node-bundle.tar", version, timeout)

	if err != nil {
		glog.Fatalf("Failed to create a dind cluster object: %v", err)
	}
	glog.Infof("Starting a cluster.")
	if err := c.Up(loadImage); err != nil {
		glog.Fatalf("Failed to wait for cluster to come up: %v", err)
	}
	glog.Infof("Cluster is initialized.")
}
