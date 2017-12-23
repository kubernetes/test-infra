/*
Copyright 2015 The Kubernetes Authors.

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
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/coreos/go-etcd/etcd"
	flag "github.com/spf13/pflag"
)

const (
	prefix         = "/kubernetes.io/network"
	server         = "http://127.0.0.1:4001"
	defaultNetwork = "/default-network.json"
	healthPort     = 8081
)

var (
	flags       = flag.NewFlagSet("a daemon to manage a flannel server.", flag.ExitOnError)
	networkPath = flags.String("network-config", defaultNetwork, "path to a json file describing network configuration.")
	etcdPrefix  = flags.String("etcd-prefix", prefix, "prefix to store network config, as understood by flannel.")
	etcdServer  = flags.String("etcd-server", server, "etcd server address.")
	healthzPort = flags.Int("healthz-port", healthPort, "port for healthz endpoint.")
)

func registerHandlers() {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		// TODO: Read from etcd, get leases from flannel server.
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})
}

func main() {
	// TODO: Add healthz and use it for flannel
	flags.Parse(os.Args)
	if *networkPath == "" {
		log.Printf("Please specify a --network-config argument.")
		return
	}
	if _, err := os.Stat(*networkPath); err != nil {
		log.Fatalf("Did not find network config file %v", *networkPath)
	}
	buff, err := ioutil.ReadFile(*networkPath)
	if err != nil {
		log.Fatalf("Unable to read network configuration: %v", err)
	}

	client := etcd.NewClient([]string{*etcdServer})
	networkBlob := string(buff)
	configPath := fmt.Sprintf("%v/%v", *etcdPrefix, "config")
	if _, err := client.Set(configPath, networkBlob, uint64(0)); err != nil {
		log.Fatalf("Unable to create network configuration: %+v", err)
	}
	log.Printf("Created network %v", networkBlob)

	log.Fatalf(fmt.Sprintf("%v", http.ListenAndServe(fmt.Sprintf(":%v", *healthzPort), nil)))
}
