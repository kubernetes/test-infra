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

package provider

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/hashicorp/terraform/helper/schema"
	"github.com/hashicorp/terraform/terraform"
	"k8s.io/kubernetes/pkg/api"
	"k8s.io/kubernetes/pkg/client/clientset_generated/release_1_4"
	"k8s.io/kubernetes/pkg/client/restclient"
	"k8s.io/kubernetes/pkg/client/unversioned/clientcmd"
	clientcmdapi "k8s.io/kubernetes/pkg/client/unversioned/clientcmd/api"
)

const (
	userAgent = "terraform-kubernetes"

	pollInterval = 10 * time.Second
	pollTimeout  = 10 * time.Minute

	configPollInterval = 100 * time.Millisecond
	configPollTimeout  = 2 * time.Minute

	resourceShutdownInterval = 1 * time.Minute
)

// Provider returns an implementation of the Kubernetes provider.
func Provider() terraform.ResourceProvider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"kubernetes_kubeconfig": resourceKubeconfig(),
			"kubernetes_cluster":    resourceCluster(),
		},

		ConfigureFunc: providerConfig,
	}
}

type configFunc func(*schema.ResourceData) (*config, error)

type config struct {
	pollInterval             time.Duration
	pollTimeout              time.Duration
	configPollInterval       time.Duration
	ConfigPollTimeout        time.Duration
	resourceShutdownInterval time.Duration

	kubeConfig *clientcmdapi.Config
	clientset  release_1_4.Interface
}

func providerConfig(d *schema.ResourceData) (interface{}, error) {
	var f configFunc = func(d *schema.ResourceData) (*config, error) {
		server := d.Get("server").(string)

		configGetter := kubeConfigGetter(d)

		clientConfig, err := clientcmd.BuildConfigFromKubeconfigGetter(server, configGetter)
		if err != nil {
			return nil, fmt.Errorf("couldn't parse the supplied config: %v", err)
		}

		clientset, err := release_1_4.NewForConfig(restclient.AddUserAgent(clientConfig, userAgent))
		if err != nil {
			return nil, fmt.Errorf("failed to initialize the cluster client: %v", err)
		}

		kubeConfig, err := configGetter()
		if err != nil {
			return nil, fmt.Errorf("couldn't parse the supplied config: %v", err)
		}

		return &config{
			pollInterval:             pollInterval,
			pollTimeout:              pollTimeout,
			configPollInterval:       configPollInterval,
			ConfigPollTimeout:        configPollTimeout,
			resourceShutdownInterval: resourceShutdownInterval,

			kubeConfig: kubeConfig,
			clientset:  clientset,
		}, nil
	}
	return f, nil
}

func resourceKubeconfig() *schema.Resource {
	return &schema.Resource{
		Create: createKubeconfig,
		Delete: deleteKubeconfig,
		Read:   readKubeconfig,

		Schema: map[string]*schema.Schema{
			"server": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Domain name or IP address of the API server",
				ForceNew:    true,
			},
			"configdata": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "kubeconfig in the serialized JSON format",
				ForceNew:    true,
			},
			"path": {
				Type:        schema.TypeString,
				Optional:    true,
				Description: "path to the kubeconfig file",
				ForceNew:    true,
			},
		},
	}
}

func createKubeconfig(d *schema.ResourceData, meta interface{}) error {
	configF := meta.(configFunc)
	cfg, err := configF(d)
	if err != nil {
		return fmt.Errorf("failed to initialize the cluster client: %v", err)
	}

	if len(cfg.kubeConfig.Clusters) != 1 || len(cfg.kubeConfig.AuthInfos) != 1 || len(cfg.kubeConfig.Contexts) != 1 {
		return fmt.Errorf("config must supplied for exactly one cluster - number of clusters: %d, number of users: %d, number of contexts: %d", len(cfg.kubeConfig.Clusters), len(cfg.kubeConfig.AuthInfos), len(cfg.kubeConfig.Contexts))
	}

	log.Printf("[DEBUG] checking for cluster components' health")
	if !poll(cfg.pollInterval, cfg.pollTimeout, allComponentsHealthy(cfg.clientset)) {
		return fmt.Errorf("cluster components never turned healthy")
	}

	po := clientcmd.NewDefaultPathOptions()
	if path, ok := d.GetOk("path"); ok {
		po.LoadingRules.ExplicitPath = path.(string)
	}

	// Retry until modifyConfig succeeds or times out.
	log.Printf("[DEBUG] updating kubeconfig")
	if !poll(cfg.configPollInterval, cfg.ConfigPollTimeout, updateConfig(po, cfg.kubeConfig)) {
		return fmt.Errorf("couldn't update kubeconfig")
	}

	// Store the ID now
	d.SetId(d.Get("server").(string))

	return nil
}

func deleteKubeconfig(d *schema.ResourceData, meta interface{}) error {
	d.SetId("")
	return nil
}

func readKubeconfig(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func resourceCluster() *schema.Resource {
	return &schema.Resource{
		Create: createCluster,
		Delete: deleteCluster,
		Read:   readCluster,

		Schema: map[string]*schema.Schema{
			"server": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Domain name or IP address of the API server",
				ForceNew:    true,
			},
			"configdata": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "kubeconfig in the serialized JSON format",
				ForceNew:    true,
			},
		},
	}
}

func createCluster(d *schema.ResourceData, meta interface{}) error {
	configF := meta.(configFunc)
	cfg, err := configF(d)
	if err != nil {
		return fmt.Errorf("failed to initialize the cluster client: %v", err)
	}

	log.Printf("[DEBUG] checking for cluster components' health")
	if !poll(cfg.pollInterval, cfg.pollTimeout, allComponentsHealthy(cfg.clientset)) {
		return fmt.Errorf("cluster components never turned healthy")
	}

	// Store the ID now
	d.SetId(d.Get("server").(string))

	return nil
}

func deleteCluster(d *schema.ResourceData, meta interface{}) error {
	configF := meta.(configFunc)
	cfg, err := configF(d)
	if err != nil {
		return fmt.Errorf("failed to initialize the cluster client: %v", err)
	}

	if err := cfg.clientset.Core().Nodes().DeleteCollection(&api.DeleteOptions{}, api.ListOptions{}); err != nil {
		return fmt.Errorf("failed to delete the nodes: %v", err)
	}

	// Block for some time to give the controllers sufficient time to
	// delete the cloud provider resources they might have acquired.
	// Only resources we are considering right now are routes installed
	// by the route controller.
	// TODO: Enumerate the resources we should wait for before returning.
	time.Sleep(cfg.resourceShutdownInterval)

	d.SetId("")

	return nil
}

func readCluster(d *schema.ResourceData, meta interface{}) error {
	return nil
}

func poll(pollInterval, pollTimeout time.Duration, cond func() (bool, error)) bool {
	interval := time.NewTicker(pollInterval)
	defer interval.Stop()
	timeout := time.NewTimer(pollTimeout)
	defer timeout.Stop()

	// Try the first time before waiting.
	if ok, err := cond(); ok {
		log.Printf("[DEBUG] condition succeeded, error: %v", err)
		return true
	} else if err != nil {
		log.Printf("[DEBUG] condition error: %v", err)
		return false
	} else {
		log.Printf("[DEBUG] condition has failed, retrying...")
	}

	for {
		select {
		case <-interval.C:
			if ok, err := cond(); ok {
				log.Printf("[DEBUG] condition succeeded, error: %v", err)
				return true
			} else if err != nil {
				log.Printf("[DEBUG] condition error: %v", err)
				return false
			} else {
				log.Printf("[DEBUG] condition has failed, retrying...")
			}
		case <-timeout.C:
			return false
		}
	}
	// Something went wrong
	log.Printf("[DEBUG] something went wrong while polling, that's all we know")
	return false
}

func allComponentsHealthy(clientset release_1_4.Interface) func() (bool, error) {
	return func() (bool, error) {
		csList, err := clientset.Core().ComponentStatuses().List(api.ListOptions{})
		if err != nil || len(csList.Items) <= 0 {
			log.Printf("[DEBUG] Listing components failed %s", err)
			return false, nil
		}
		for _, cs := range csList.Items {
			if !(len(cs.Conditions) > 0 && cs.Conditions[0].Type == "Healthy" && cs.Conditions[0].Status == "True") {
				log.Printf("[DEBUG] %s isn't healthy. Conditions: %+v", cs.Name, cs.Conditions)
				return false, nil
			}
		}
		return true, nil
	}
}

func kubeConfigGetter(d *schema.ResourceData) clientcmd.KubeconfigGetter {
	return func() (*clientcmdapi.Config, error) {
		kubeConfigStr := d.Get("configdata").(string)
		return clientcmd.Load([]byte(kubeConfigStr))
	}
}

func updateConfig(configAccess clientcmd.ConfigAccess, suppliedConfig *clientcmdapi.Config) func() (bool, error) {
	return func() (bool, error) {
		err := modifyConfig(configAccess, suppliedConfig)
		if err != nil {
			// TODO: We are relying too much on the fact that this error is going
			// to be an *os.PathError returned by file locking mechanism. This is
			// dangerous. Try to introduce a specific error for this in
			// "k8s.io/kubernetes/pkg/client/unversioned/clientcmd" package.
			if os.IsExist(err) {
				return false, nil
			}
			return false, fmt.Errorf("couldn't update kubeconfig: %v", err)
		}
		return true, nil
	}
}

func modifyConfig(configAccess clientcmd.ConfigAccess, suppliedConfig *clientcmdapi.Config) error {
	config, err := configAccess.GetStartingConfig()
	if err != nil {
		return err
	}

	for name, authInfo := range suppliedConfig.AuthInfos {
		initial, ok := config.AuthInfos[name]
		if !ok {
			initial = clientcmdapi.NewAuthInfo()
		}
		modifiedAuthInfo := *initial

		var setToken, setBasic bool

		if len(authInfo.ClientCertificate) > 0 {
			modifiedAuthInfo.ClientCertificate = authInfo.ClientCertificate
		}
		if len(authInfo.ClientCertificateData) > 0 {
			modifiedAuthInfo.ClientCertificateData = authInfo.ClientCertificateData
		}

		if len(authInfo.ClientKey) > 0 {
			modifiedAuthInfo.ClientKey = authInfo.ClientKey
		}
		if len(authInfo.ClientKeyData) > 0 {
			modifiedAuthInfo.ClientKeyData = authInfo.ClientKeyData
		}

		if len(authInfo.Token) > 0 {
			modifiedAuthInfo.Token = authInfo.Token
			setToken = len(modifiedAuthInfo.Token) > 0
		}

		if len(authInfo.Username) > 0 {
			modifiedAuthInfo.Username = authInfo.Username
			setBasic = setBasic || len(modifiedAuthInfo.Username) > 0
		}
		if len(authInfo.Password) > 0 {
			modifiedAuthInfo.Password = authInfo.Password
			setBasic = setBasic || len(modifiedAuthInfo.Password) > 0
		}

		// If any auth info was set, make sure any other existing auth types are cleared
		if setToken || setBasic {
			if !setToken {
				modifiedAuthInfo.Token = ""
			}
			if !setBasic {
				modifiedAuthInfo.Username = ""
				modifiedAuthInfo.Password = ""
			}
		}
		config.AuthInfos[name] = &modifiedAuthInfo
	}

	for name, cluster := range suppliedConfig.Clusters {
		initial, ok := config.Clusters[name]
		if !ok {
			initial = clientcmdapi.NewCluster()
		}
		modifiedCluster := *initial

		if len(cluster.Server) > 0 {
			modifiedCluster.Server = cluster.Server
		}
		if cluster.InsecureSkipTLSVerify {
			modifiedCluster.InsecureSkipTLSVerify = cluster.InsecureSkipTLSVerify
			// Specifying insecure mode clears any certificate authority
			if modifiedCluster.InsecureSkipTLSVerify {
				modifiedCluster.CertificateAuthority = ""
				modifiedCluster.CertificateAuthorityData = nil
			}
		}
		if len(cluster.CertificateAuthorityData) > 0 {
			modifiedCluster.CertificateAuthorityData = cluster.CertificateAuthorityData
			modifiedCluster.InsecureSkipTLSVerify = false
		}
		if len(cluster.CertificateAuthority) > 0 {
			modifiedCluster.CertificateAuthority = cluster.CertificateAuthority
			modifiedCluster.InsecureSkipTLSVerify = false
		}
		config.Clusters[name] = &modifiedCluster
	}

	for name, context := range suppliedConfig.Contexts {
		initial, ok := config.Contexts[name]
		if !ok {
			initial = clientcmdapi.NewContext()
		}
		modifiedContext := *initial

		if len(context.Cluster) > 0 {
			modifiedContext.Cluster = context.Cluster
		}
		if len(context.AuthInfo) > 0 {
			modifiedContext.AuthInfo = context.AuthInfo
		}
		if len(context.Namespace) > 0 {
			modifiedContext.Namespace = context.Namespace
		}
		config.Contexts[name] = &modifiedContext
	}

	if err := clientcmd.ModifyConfig(configAccess, *config, true); err != nil {
		return err
	}

	return nil
}
