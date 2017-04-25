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
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
)

var (
	kfKubefedConfig kubefedConfig
	kfClusterCount  int
)

func init() {
	flag.IntVar(&kfClusterCount, "kops-federation-cluster-count", 3, "(kops-federation only) how many underlying clusters to provision for the federation. Federation control plane will be deployed to \"cluster-0\"")

	flag.StringVar(&kfKubefedConfig.federationName, "kops-federation-name", "kops-federation", "(kops-federation only) name of kops federation kubecontext and A record. Underlying cluster names will be generated from this as well.")
	flag.StringVar(&kfKubefedConfig.federationNamespace, "kops-federation-namespace", "kops-federation", "(kops-federation only) namespace to deploys federation control plane to in underlying cluster")
	flag.StringVar(&kfKubefedConfig.dnsZoneName, "kops-federation-dns-zone-name", "", "(kops-federation only) name of DNS zone to use for federated services")
	flag.StringVar(&kfKubefedConfig.dnsProvider, "kops-federation-dns-provider", "", "(kops-federation only) dns provider to user for federated services.")
	flag.StringVar(&kfKubefedConfig.federationKubeContext, "kops-federation-kube-context", "kops-federation", "(kops-federation only) name of federation kube context.")
	flag.StringVar(&kfKubefedConfig.kubeRegistry, "kops-federation-hyperkube-registry", "gcr.io/k8s-jkns-e2e-gce-federation", "(kops-federation only) name of federation kube context.")
}

type kopsFederation struct {
	underlyingClusters []*kopsFedCluster
	kubefed            kubefedConfig
}

type clusterResult struct {
	cluster *kopsFedCluster
	err     error
}
type kopsFedCluster struct {
	*kops
	clusterName string
}

func NewKopsFederation() (*kopsFederation, error) {
	if kfClusterCount < 1 {
		return nil, fmt.Errorf("federation-cluster-count must be >= 1")
	}
	f, err := ioutil.TempFile("", "kops-federation-kubecfg")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	kubecfg := f.Name()
	if err := f.Chmod(0600); err != nil {
		return nil, err
	}

	kf := &kopsFederation{}
	kf.underlyingClusters = make([]*kopsFedCluster, kfClusterCount)
	kf.kubefed = kfKubefedConfig
	for i := 0; i < kfClusterCount; i++ {
		var err error
		var k kopsFedCluster
		k.kops, err = NewKops()
		if err != nil {
			return nil, err
		}
		// The underlying kops clusters uses the same DNS zone as the federation control plane
		// Could very well provide distinct dns zone name for the two
		k.clusterName = fmt.Sprintf("%s-%d", kf.kubefed.federationName, i)
		k.cluster = fmt.Sprintf("%s.%s", k.clusterName, kf.kubefed.dnsZoneName)
		k.kubecfg = kubecfg

		//Scope underlying cluster kops working directories
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("could not get pwd: %v", err)
		}
		acwd, err := filepath.Abs(cwd)
		if err != nil {
			return nil, fmt.Errorf("failed to convert %s to an absolute path: %v", cwd, err)
		}

		k.workdir = filepath.Join(acwd, fmt.Sprintf("%s-kops-workspace", k.clusterName))
		if err := os.MkdirAll(k.workdir, 0700); err != nil {
			return nil, fmt.Errorf("error createing working directory for %s: %v", k.clusterName, err)
		}

		//Scope underlying cluster state store
		stateURL, err := url.Parse(*kopsState)
		if err != nil {
			return nil, fmt.Errorf("error parsing kops state location: %v", err)
		}
		stateURL.Path = filepath.Join(stateURL.Path, k.clusterName)
		k.state = stateURL.String()
		kf.underlyingClusters[i] = &k
	}
	// Federation control plane is deployed to the "first" underyling cluster
	kf.kubefed.hostClusterContext = kf.underlyingClusters[0].cluster

	if err := os.Setenv("KUBECONFIG", kubecfg); err != nil {
		return nil, err
	}
	if err := os.Setenv("FEDERATION", "true"); err != nil {
		return nil, err
	}

	return kf, nil
}

func (kf kopsFederation) Up() error {
	completionChan := make(chan *clusterResult)
	for _, k := range kf.underlyingClusters {
		go func(k *kopsFedCluster) {
			var err error
			if err = k.Up(); err != nil {
				err = fmt.Errorf("error bringing up underlying cluster %s : %v\n", k.cluster, err)
			}
			completionChan <- &clusterResult{
				cluster: k,
				err:     err,
			}
		}(k)
	}
	errStr := ""
	for _ = range kf.underlyingClusters {
		status := <-completionChan
		if status.err != nil {
			errStr += fmt.Sprintf("\n%s %v", status.cluster.clusterName, status.err)
		} else {
			status.cluster.SetupKubecfg()
		}
	}
	if len(errStr) != 0 {
		return errors.New(errStr)
	}
	if err := kf.kubefed.FedUp(); err != nil {
		return fmt.Errorf("error bringing up federation control plane: %v", err)
	}
	for _, k := range kf.underlyingClusters {
		if err := kf.kubefed.FedJoinCluster(k.clusterName, k.cluster); err != nil {
			return fmt.Errorf("error joining cluster %+v to federation: %v", k, err)
		}
	}
	return nil
}

func (kf kopsFederation) IsUp() error {
	errStr := ""
	for _, k := range kf.underlyingClusters {
		if err := useKubeContext(k.cluster); err != nil {
			return err
		}
		if err := k.IsUp(); err != nil {
			errStr += fmt.Sprintf("%s : %v\n", k.cluster, err)
		}
	}

	if len(errStr) != 0 {
		return fmt.Errorf("underlying clusters not up:\n%s\n", errStr)
	}

	clusterCount, err := kf.kubefed.clusterCount()
	if err != nil {
		return err
	}
	if clusterCount != len(kf.underlyingClusters) {
		return fmt.Errorf("observed cluster count %d does not match desired count %d", clusterCount, len(kf.underlyingClusters))
	}

	// We don't want to leave current context pointing at federation control plane
	// Has potential to make e2e testing framework angry
	return useKubeContext(kf.kubefed.hostClusterContext)
}

func (kf kopsFederation) SetupKubecfg() error {
	for _, k := range kf.underlyingClusters {
		if err := k.SetupKubecfg(); err != nil {
			return fmt.Errorf("error setting up kubeconfig for cluster %s: %v", k.cluster, err)
		}
	}

	if err := os.Setenv("FEDERATION_KUBE_CONTEXT", kf.kubefed.federationKubeContext); err != nil {
		return err
	}
	if err := os.Setenv("FEDERATION_NAMESPACE", kf.kubefed.federationNamespace); err != nil {
		return err
	}

	// Due to the way the e2e testing framework works, we have to set the kube-context to one of the underlying clusters. The framework will take care of explictly addressing the federation kube context
	return useKubeContext(kf.kubefed.hostClusterContext)
}

func (kf kopsFederation) Down() error {
	errStr := ""
	for _, k := range kf.underlyingClusters {
		if err := kf.kubefed.FedUnjoinCluster(k.clusterName, k.cluster); err != nil {
			errStr += fmt.Sprintf("error unjoining cluster: %v", err)
		}
	}
	if err := kf.kubefed.FedCleanup(); err != nil {
		errStr += fmt.Sprintf("\n%v", err)
	}

	completionChan := make(chan *clusterResult)
	for _, k := range kf.underlyingClusters {
		go func(k *kopsFedCluster) {
			var errStr string
			if err := k.Down(); err != nil {
				errStr += fmt.Sprintf("\nerror bringing down cluster %s down: %v", k.cluster, err)
			}

			status := &clusterResult{cluster: k}
			if errStr != "" {
				status.err = errors.New(errStr)
			}
			completionChan <- status
		}(k)
	}

	clusterErrorCnt := 0
	for _ = range kf.underlyingClusters {
		status := <-completionChan
		if status.err != nil {
			errStr += fmt.Sprintf("\n%+v", status)
			clusterErrorCnt++
		}
	}

	if len(errStr) != 0 {
		if clusterErrorCnt == 0 {
			// If underlying clusters are destroyed, federation teardown errors are non-terminal
			fmt.Printf("Underlying clusters successfully destroyed. Will ignore errors bringing down federation control plane: %s", errStr)
			return nil
		}
		return errors.New(errStr)
	}

	return nil
}
