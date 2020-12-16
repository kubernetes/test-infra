// +build e2etest
/*
Copyright 2020 The Kubernetes Authors.

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

package e2e

import (
	"crypto/rand"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"os/user"
	"path"
	"strings"
	"testing"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	prow "k8s.io/test-infra/prow/client/clientset/versioned"
)

const (
	testpodNamespace = "test-pods"
)

var clusterContext = flag.String("cluster", "kind-kind", "The context of cluster to use for test")

func getClusterContext() string {
	return *clusterContext
}

func getDefaultKubeconfig(cfg string) string {
	if cfg := strings.TrimSpace(cfg); cfg != "" {
		return cfg
	}
	defaultKubeconfig := os.Getenv("KUBECONFIG")

	// If KUBECONFIG env var isn't set then look for $HOME/.kube/config
	if defaultKubeconfig == "" {
		if usr, err := user.Current(); err == nil {
			defaultKubeconfig = path.Join(usr.HomeDir, ".kube/config")
		}
	}
	return defaultKubeconfig
}

func NewClients(configPath, clusterName string) (*kubernetes.Clientset, *prow.Clientset, error) {
	cfg, err := BuildClientConfig(getDefaultKubeconfig(configPath), clusterName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed create rest config: %v", err)
	}
	c, err := NewClientsFromConfig(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("failed creating kubernetes client: %v", err)
	}
	pc, err := prow.NewForConfig(cfg)
	return c, pc, err
}

func NewClientsFromConfig(cfg *rest.Config) (*kubernetes.Clientset, error) {
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}

	return kubeClient, nil
}

func BuildClientConfig(kubeConfigPath, clusterName string) (*rest.Config, error) {
	overrides := clientcmd.ConfigOverrides{}
	// Override the cluster name if provided.
	if clusterName != "" {
		overrides.Context.Cluster = clusterName
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeConfigPath},
		&overrides).ClientConfig()
}

// RandomString generates random string of 32 characters in length, and fail if it failed
func RandomString(t *testing.T) string {
	b := make([]byte, 512)
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("failed to generate random: %v", err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b[:]))[:32]
}
