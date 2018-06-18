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
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
	"k8s.io/test-infra/dind/pkg/cluster-up/cluster"
	"k8s.io/test-infra/dind/pkg/cluster-up/options"
)

const (
	masterConfigDir = "/var/kubernetes"
	kubecfg         = "/var/kubernetes/admin.conf"
	timeout         = time.Duration(5) * time.Minute
)

// dockerImageVersion reads the version from the metadata stored in the container.
func dockerImageVersion() (string, error) {
	f, err := os.Open("/docker_version")
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		return scanner.Text(), nil
	}
	return "", fmt.Errorf("empty version file")
}

func nonLocalUnicastAddr(intfName string) (string, error) {
	// Finds a non-localhost unicast address on the given interface.
	intf, err := net.InterfaceByName(intfName)
	if err != nil {
		return "", err
	}
	addrs, err := intf.Addrs()
	if err != nil {
		return "", err
	}
	for _, addr := range addrs {
		if !strings.HasPrefix("127", addr.String()) {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				return "", err
			}

			return ip.String(), nil
		}
	}

	return "", fmt.Errorf("no non-localhost unicast addresses found on interface %s", intfName)
}

func main() {
	// Discard the error, because flag.CommandLine is already set to ExitOnError.
	o, _ := options.New(flag.CommandLine, os.Args[1:])
	defer glog.Flush()

	// Dynamically lookup any arguments that weren't provided.
	if o.Version == "" {
		v, err := dockerImageVersion()
		if err != nil {
			glog.Fatalf("Failed to load docker image version: %v", err)
		}
		o.Version = v
	}

	if o.ProxyAddr == "" {
		p, err := nonLocalUnicastAddr("eth0")
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
