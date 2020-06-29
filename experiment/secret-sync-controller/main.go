package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	// apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog"
	"k8s.io/test-infra/experiment/secret-sync-controller/client"
	"os"
	"time"
)

type options struct {
	configPath   string
	testSetup    string
	runOnce      bool
	resyncPeriod int64
}

func (o *options) Validate() error {
	if o.configPath == "" {
		return errors.New("required flag --config-path was unset")
	}
	return nil
}

func gatherOptions() options {
	o := options{}
	flag.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	flag.StringVar(&o.testSetup, "test-setup", "", "Path to test-setup.yaml.")
	flag.BoolVar(&o.runOnce, "run-once", false, "Sync once instead of continuous loop.")
	flag.Int64Var(&o.resyncPeriod, "period", 1000, "Resync period in milliseconds.")
	flag.Parse()
	return o
}

type SecretSyncController struct {
	Client       client.Interface
	Config       *SecretSyncConfig
	RunOnce      bool
	ResyncPeriod time.Duration
	// TODO: in-struct looger?
}

func main() {
	o := gatherOptions()

	// prepare clients
	k8sClientset, err := client.NewK8sClientset()
	if err != nil {
		klog.Errorf("New kubernetes client failed: %s", err)
	}
	secretManagerClient, err := client.NewSecretManagerClient(context.Background())
	if err != nil {
		klog.Errorf("New Secret Manager client failed: %s", err)
	}
	clientInterface := &client.Client{
		K8sClientset:        *k8sClientset,
		SecretManagerClient: *secretManagerClient,
	}

	// prepare config
	secretSyncConfig, err := LoadConfig(o.configPath)
	if err != nil {
		klog.Errorf("Load config failed: %s", err)
	}

	controller := &SecretSyncController{
		Client:       clientInterface,
		Config:       secretSyncConfig,
		RunOnce:      o.runOnce,
		ResyncPeriod: time.Duration(o.resyncPeriod) * time.Millisecond,
	}

	err = controller.ValidateAccess()
	if err != nil {
		klog.Error(err)
	}

	var stopChan <-chan struct{}
	controller.Start(stopChan)

}

func (c *SecretSyncController) Start(stopChan <-chan struct{}) error {
	runChan := make(chan struct{})

	go func() {
		for {
			runChan <- struct{}{}
			time.Sleep(c.ResyncPeriod)
		}
	}()

	for {
		select {
		case <-stopChan:
			klog.Info("Stop signal received. Quitting...")
			return nil
		case <-runChan:
			c.SyncAll()
			if c.RunOnce {
				c.Client.CleanupKubernetesSecrets("ns1")
				return nil
			}
		}
	}
}

// ValidateAccess validates the required access for each sync pair.
// If finds an invlaid access, return an error and delete that sync pair.
func (c *SecretSyncController) ValidateAccess() error {
	invalidIndices := make(map[int]bool)

	for i, spec := range c.Config.Specs {
		// ping source secret
		_, srcErr := c.Client.GetSecretManagerSecret(spec.Source.Project, spec.Source.Secret)
		if srcErr != nil {
			invalidIndices[i] = true
			klog.Error(srcErr)
		}
		// ping destination namespace
		destErr := c.Client.ValidateKubernetesNamespace(spec.Destination.Namespace)
		if destErr != nil {
			invalidIndices[i] = true
			klog.Error(destErr)
		}
	}

	// if there is any invalid spec to remove
	if len(invalidIndices) != 0 {
		validSpecs := []SecretSyncSpec{}
		for i := range c.Config.Specs {
			if invalidIndices[i] {
				continue
			}
			validSpecs = append(validSpecs, c.Config.Specs[i])
		}
		c.Config.Specs = validSpecs
	}

	return nil
}

func (c *SecretSyncController) SyncAll() error {
	for _, spec := range c.Config.Specs {
		updated, err := c.Sync(spec)
		if err != nil {
			klog.Errorf("Secret sync failed for %s: %s", spec, err)
		}
		if updated {
			klog.Infof("Secret %s synced from %s", spec.Destination, spec.Source)
		}
	}
	return nil
}

func (c *SecretSyncController) Sync(spec SecretSyncSpec) (bool, error) {
	// get source secret
	srcData, err := c.Client.GetSecretManagerSecret(spec.Source.Project, spec.Source.Secret)
	if err != nil {
		return false, err
	}

	//checks if the K8s namespace exists
	err = c.Client.ValidateKubernetesNamespace(spec.Destination.Namespace)
	if err != nil {
		return false, err
	}

	// get destination secret
	destData, err := c.Client.GetKubernetesSecret(spec.Destination.Namespace, spec.Destination.Secret, spec.Destination.Key)

	if err != nil {
		return false, err
	}
	// update destination secret
	if bytes.Equal(srcData, destData) {
		return false, nil
	}
	// update destination secret value
	// inserts a key-value pair if spec.Destination does not exist yet
	patchErr := c.Client.UpsertKubernetesSecret(spec.Destination.Namespace, spec.Destination.Secret, spec.Destination.Key, srcData)
	if patchErr != nil {
		return false, patchErr
	}

	return true, nil
}

// LoadConfig loads from a yaml file and returns the structure
func LoadConfig(config string) (*SecretSyncConfig, error) {
	stat, err := os.Stat(config)
	if err != nil {
		return nil, err
	}

	if stat.IsDir() {
		return nil, fmt.Errorf("config cannot be a dir - %s", config)
	}

	yamlFile, err := ioutil.ReadFile(config)
	if err != nil {
		return nil, fmt.Errorf("Error reading YAML file: %s\n", err)
	}

	syncConfig := SecretSyncConfig{}
	err = yaml.Unmarshal(yamlFile, &syncConfig)
	if err != nil {
		return nil, fmt.Errorf("Error reading YAML file: %s\n", err)
	}

	return &syncConfig, nil
}
