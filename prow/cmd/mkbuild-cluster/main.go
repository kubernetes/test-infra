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
	"io/ioutil"
	"os"
	"os/exec"
	"strings"

	"github.com/sirupsen/logrus"
	coreapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/test-infra/prow/kube"
	"sigs.k8s.io/yaml"
)

const (
	useClientCertEnv = "CLOUDSDK_CONTAINER_USE_CLIENT_CERTIFICATE"
)

type options struct {
	account       string
	alias         string
	changeContext bool
	cluster       string
	getClientCert bool
	overwrite     bool
	printEntry    bool
	printData     bool
	project       string
	skipCheck     bool
	zone          string
}

type describe struct {
	Auth     describeAuth `json:"masterAuth"`
	Endpoint string       `json:"endpoint"`
}

type describeAuth struct {
	ClientCertificate    []byte `json:"clientCertificate"`
	ClientKey            []byte `json:"clientKey"`
	ClusterCACertificate []byte `json:"clusterCaCertificate"`
}

func parseOptions() options {
	var o options
	if err := o.parseArgs(flag.CommandLine, os.Args[1:]); err != nil {
		logrus.Fatalf("Invalid flags: %v", err)
	}
	return o
}

func (o *options) parseArgs(flags *flag.FlagSet, args []string) error {
	flags.StringVar(&o.account, "account", "", "use this account to describe --cluster")
	flags.StringVar(&o.alias, "alias", "", "the --build-cluster alias to add")
	flags.StringVar(&o.cluster, "cluster", "", "the GKE cluster to describe")
	flags.StringVar(&o.project, "project", "", "the GKE project to describe")
	flags.StringVar(&o.zone, "zone", "", "the GKE zone to describe")
	flags.BoolVar(&o.printData, "print-file", false, "print the file outside of the configmap secret")
	flags.BoolVar(&o.printEntry, "print-entry", false, "print the new entry without appending to existing ones at stdin")
	flags.BoolVar(&o.getClientCert, "get-client-cert", false, fmt.Sprintf("first get-credentials for the cluster using %s=True", useClientCertEnv))
	flags.BoolVar(&o.changeContext, "change-context", false, "allow --get-client-cert to change kubectl config current-context")
	flags.BoolVar(&o.skipCheck, "skip-check", false, "skip validating the creds work in a client")
	switch err := flags.Parse(args); {
	case err != nil:
		return err
	case o.cluster == "":
		return errors.New("--cluster required")
	case o.project == "":
		return errors.New("--project required")
	case o.zone == "":
		return errors.New("--zone required")
	case o.alias == "":
		return fmt.Errorf("--alias required (use %q for default)", kube.DefaultClusterAlias)
	}
	return nil
}

func main() {
	// Gather options from flags
	o := parseOptions()
	if err := do(o); err != nil {
		logrus.Fatalf("Failed: %v", err)
	}
}

// useContext calls kubectl config use-context ctx
func useContext(o options, ctx string) error {
	_, cmd := command("kubectl", "config", "use-context", ctx)
	return cmd.Run()
}

// currentContext returns kubectl config current-context
func currentContext(o options) (string, error) {
	_, cmd := command("kubectl", "config", "current-context")
	b, err := cmd.Output()
	return strings.TrimSpace(string(b)), err
}

// getCredentials calls gcloud container clusters get-credentials, usually preserving currentContext()
func getCredentials(o options) error {
	if !o.changeContext {
		cur, err := currentContext(o)
		if err != nil {
			return fmt.Errorf("read current-context: %v", err)
		}
		defer useContext(o, cur)
	}

	// TODO(fejta): we ought to update kube.Client to support modern auth methods.
	// More info: https://github.com/kubernetes/kubernetes/issues/30617
	old, set := os.LookupEnv(useClientCertEnv)
	if set {
		defer os.Setenv(useClientCertEnv, old)
	}
	if err := os.Setenv("CLOUDSDK_CONTAINER_USE_CLIENT_CERTIFICATE", "True"); err != nil {
		return fmt.Errorf("failed to set %s: %v", useClientCertEnv, err)
	}
	args, cmd := command(
		"gcloud", "container", "clusters", "get-credentials", o.cluster,
		"--project", o.project,
		"--zone", o.zone,
	)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %v", strings.Join(args, " "), err)
	}
	return nil
}

// command creates an exec.Cmd with Stderr piped to os.Stderr and returns the args
func command(bin string, args ...string) ([]string, *exec.Cmd) {
	cmd := exec.Command(bin, args...)
	cmd.Stderr = os.Stderr
	return append([]string{bin}, args...), cmd
}

// getAccount returns gcloud config get-value core/account
func getAccount() (string, error) {
	args, cmd := command("gcloud", "config", "get-value", "core/account")
	b, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(b)), nil
}

// setAccount calls gcloud config set core/account
func setAccount(account string) error {
	_, cmd := command("gcloud", "config", "set", "core/account", account)
	return cmd.Run()
}

// describeCluster returns details from gcloud container clusters describe.
func describeCluster(o options) (*describe, error) {
	if o.account != "" {
		act, err := getAccount()
		if err != nil {
			return nil, fmt.Errorf("get current account: %v", err)
		}
		defer setAccount(act)
		if err = setAccount(o.account); err != nil {
			return nil, fmt.Errorf("set account %s: %v", o.account, err)
		}
	}
	args, cmd := command(
		"gcloud", "container", "clusters", "describe", o.cluster,
		"--project", o.project,
		"--zone", o.zone,
		"--format=yaml",
	)
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: %v", strings.Join(args, " "), err)
	}
	var d describe
	if yaml.Unmarshal(data, &d); err != nil {
		return nil, fmt.Errorf("unmarshal gcloud: %v", err)
	}

	if d.Endpoint == "" {
		return nil, errors.New("empty endpoint")
	}
	if len(d.Auth.ClusterCACertificate) == 0 {
		return nil, errors.New("empty clusterCaCertificate")
	}

	if len(d.Auth.ClientKey) == 0 {
		return nil, errors.New("empty clientKey, consider running with --get-client-cert")
	}
	if len(d.Auth.ClientCertificate) == 0 {
		return nil, errors.New("empty clientCertificate, consider running with --get-client-cert")
	}

	return &d, nil
}

// do will get creds for the specified cluster and add them to the stdin secret
func do(o options) error {
	// Refresh credentials if requested
	if o.getClientCert {
		if err := getCredentials(o); err != nil {
			return fmt.Errorf("get client cert: %v", err)
		}
	}
	// Create the new cluster entry
	d, err := describeCluster(o)
	if err != nil {
		return fmt.Errorf("describe auth: %v", err)
	}
	newCluster := kube.Cluster{
		Endpoint:             "https://" + d.Endpoint,
		ClusterCACertificate: d.Auth.ClusterCACertificate,
		ClientKey:            d.Auth.ClientKey,
		ClientCertificate:    d.Auth.ClientCertificate,
	}

	// Try to use this entry
	if !o.skipCheck {
		cfg := &clientcmdapi.Config{
			Clusters: map[string]*clientcmdapi.Cluster{
				o.alias: {
					Server:                   newCluster.Endpoint,
					CertificateAuthorityData: newCluster.ClusterCACertificate,
				},
			},
			AuthInfos: map[string]*clientcmdapi.AuthInfo{
				o.alias: {
					ClientCertificateData: newCluster.ClientCertificate,
					ClientKeyData:         newCluster.ClientKey,
				},
			},
			Contexts: map[string]*clientcmdapi.Context{
				o.alias: {
					Cluster:  o.alias,
					AuthInfo: o.alias,
				},
			},
		}

		contextCfg, err := clientcmd.NewNonInteractiveClientConfig(*cfg, o.alias, &clientcmd.ConfigOverrides{}, nil).ClientConfig()
		if err != nil {
			return fmt.Errorf("create %s client: %v", o.alias, err)
		}
		client, err := kubernetes.NewForConfig(contextCfg)
		if err != nil {
			return fmt.Errorf("could not create kubernetes client: %v", err)
		}
		if _, err = client.CoreV1().Pods("kube-system").List(metav1.ListOptions{}); err != nil {
			return fmt.Errorf("authenticated client could not list pods in the kube-system namespace: %v", err)
		}
	}

	// Just print this entry if requested
	if o.printEntry {
		data, err := kube.MarshalClusterMap(map[string]kube.Cluster{o.alias: newCluster})
		if err != nil {
			return fmt.Errorf("marshal %s: %v", o.alias, err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Append the new entry to the current secret

	// First read in the secret from stdin
	b, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %v", err)
	}
	var s coreapi.Secret
	if err := yaml.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("unmarshal stdin: %v", err)
	}

	// Now decode the {alias: cluster} map and print out current keys
	clusters, err := kube.UnmarshalClusterMap(s.Data["cluster"])
	if err != nil {
		return fmt.Errorf("unmarshal secret: %v", err)
	}
	var existing []string
	for a := range clusters {
		existing = append(existing, a)
	}
	logrus.Infof("Existing clusters: %s", strings.Join(existing, ", "))

	// Add new key
	_, ok := clusters[o.alias]
	if ok && !o.overwrite {
		return fmt.Errorf("cluster %s already exists", o.alias)
	}
	clusters[o.alias] = newCluster
	logrus.Infof("New cluster: %s", o.alias)

	// Marshal the {alias: cluster} map back into secret data
	data, err := kube.MarshalClusterMap(clusters)
	if err != nil {
		return fmt.Errorf("marshal clusters: %v", err)
	}

	if o.printData { // Just print the data outside of the secret
		fmt.Println(string(data))
		return nil
	}

	// Output the new secret
	s.Data["cluster"] = data
	buf, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal secret: %v", err)
	}
	fmt.Println(string(buf))
	return nil
}
