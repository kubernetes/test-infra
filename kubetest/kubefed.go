/*
Copyright 2017 The Kubernetes Authors.

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
	"fmt"
	"log"
	"os/exec"
	"strings"
	"time"
)

const (
	kubefed = "./federation/develop/kubefed.sh"
	kubectl = "./cluster/kubectl.sh"
)

var (
	federationUpTimeout time.Duration
)

func init() {
	flag.DurationVar(&federationUpTimeout, "federation-up-timeout", 5*time.Minute, "(federation only) how long to wait for federation control plane to come up")
}

type kubefedConfig struct {
	kubeRegistry          string
	dnsProvider           string
	dnsZoneName           string
	federationName        string
	federationNamespace   string
	federationKubeContext string
	hostClusterContext    string
}

func (kf kubefedConfig) FedUp() error {
	hkVersion, err := hyperkubeVersion()
	if err != nil {
		return err
	}
	kfInitCmd := exec.Command(
		kubefed, "init",
		kf.federationKubeContext,
		"--federation-system-namespace="+kf.federationNamespace,
		"--host-cluster-context="+kf.hostClusterContext,
		"--dns-zone-name="+kf.dnsZoneName,
		"--dns-provider="+kf.dnsProvider,
		fmt.Sprintf("--image=%s/hyperkube-amd64:%s", kf.kubeRegistry, hkVersion),
		"--apiserver-arg-overrides=\"--storage-backend=etcd2\"",
		"--apiserver-enable-basic-auth=true",
		"--apiserver-enable-token-auth=true",
		"--apiserver-arg-overrides=\"--v=4\"",
		"--controllermanager-arg-overrides=\"--v=4\"",
	)
	if err := finishRunning(kfInitCmd); err != nil {
		return err
	}
	useKubeContext(kf.federationKubeContext)

	if err := retryFinishRunning(exec.Command("./cluster/kubectl.sh", "get", "clusters"), federationUpTimeout); err != nil {
		return err
	}
	// https://kubernetes.io/docs/tutorials/federation/set-up-cluster-federation-kubefed/#deploying-a-federation-control-plane
	// According to the docs for kubefed, this may be necessary due to a bug
	// This default/ns check logic should eventually go away
	log.Printf("checking federation api server for ns/default...")
	if err := finishRunning(exec.Command("./cluster/kubectl.sh", "get", "ns/default")); err == nil {
		log.Printf("federation api endpoint is accessible and ns/default exists!")
	} else {
		log.Printf("kubectl get ns/default failed: %v", err)
		_ = finishRunning(exec.Command("./cluster/kubectl.sh", "create", "ns", "default"))
	}

	return nil
}

func (kf kubefedConfig) FedJoinCluster(clusterName, kubeContext string) error {
	if err := useKubeContext(kf.federationKubeContext); err != nil {
		return err
	}
	kfJoinCmd := exec.Command(
		kubefed, "join", clusterName,
		"--cluster-context="+kubeContext,
		"--federation-system-namespace="+kf.federationNamespace,
		"--host-cluster-context="+kf.hostClusterContext,
		"--context="+kf.federationKubeContext,
		"--secret-name="+clusterName,
	)
	return finishRunning(kfJoinCmd)
}

func (kf kubefedConfig) FedUnjoinCluster(clusterName, kubeContext string) error {
	if err := useKubeContext(kf.federationKubeContext); err != nil {
		return err
	}
	kfUnjoinCmd := exec.Command(
		kubefed, "unjoin", clusterName,
		"--host-cluster-context="+kf.hostClusterContext,
		"--federation-system-namespace="+kf.federationNamespace,
	)
	return finishRunning(kfUnjoinCmd)
}

func (kf kubefedConfig) FedCleanup() error {
	useKubeContext(kf.hostClusterContext)
	var errStr string
	if err := finishRunning(exec.Command(kubectl, "-n", kf.federationNamespace,
		"delete",
		"pvc,pv,pods,svc,rc,deployment,secret",
		"--all",
	)); err != nil {
		errStr += fmt.Sprintf("\n%v", err)
	}
	if err := finishRunning(exec.Command(
		kubectl, "-n", kf.federationNamespace,
		"delete",
		"pods,svc,rc,deployment,secret",
		"-lapp=federated-cluster",
	)); err != nil {
		errStr += fmt.Sprintf("\n%v", err)
	}

	if err := finishRunning(exec.Command(kubectl, "delete", "ns", kf.federationNamespace)); err != nil {
		errStr += fmt.Sprintf("\n%v", err)
	}
	for {
		if err := finishRunning(exec.Command(kubectl, "get", "ns", kf.federationNamespace)); err == nil {
			time.Sleep(5 * time.Second)
		} else {
			break
		}
	}
	if len(errStr) > 0 {
		return fmt.Errorf("errors encountered in federation cleanup: %s", errStr)
	}
	return nil
}

func (kf kubefedConfig) clusterCount() (int, error) {
	if err := useKubeContext(kf.federationKubeContext); err != nil {
		return -1, err
	}
	o, err := output(exec.Command(kubectl, "get", "cluster", "--no-headers"))
	if err != nil {
		log.Printf("kubectl get clusters failed: %s\n%s", WrapError(err).Error(), string(o))
		return -1, err
	}
	stdout := strings.TrimSpace(string(o))
	log.Printf("Federation clusters:\n%s", stdout)
	return len(strings.Split(stdout, "\n")), nil
}
