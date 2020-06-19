package main

// configuration and sync-pair defination

import (
	"context"
	"fmt"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Structs for configuration
type SecretSyncConfig struct {
	Specs []SecretSyncSpec `yaml:"specs"`
}
type SecretSyncSpec struct {
	// the target can be either a K8s secret or a SecretManager secret
	Source      TargetSpec `yaml:"source"`
	Destination TargetSpec `yaml:"destination"`
}
type TargetSpec struct {
	// assert that one of the two should be `nil`
	Kubernetes    *KubernetesSpec    `yaml:"kubernetes,omitempty"`
	SecretManager *SecretManagerSpec `yaml:"secretManager,omitempty"`
}
type KubernetesSpec struct {
	Namespace string   `yaml:"namespace"`
	Secret    string   `yaml:"secret,omitempty"`
	DenyList  []string `yaml:"denyList,omitempty"`
}
type SecretManagerSpec struct {
	Project  string   `yaml:"project"`
	Secret   string   `yaml:"secret,omitempty"`
	DenyList []string `yaml:"denyList,omitempty"`
}

// structs for client interface
type ClientInterface interface {
	UpdatedVersion(Target) (bool, error)
}
type Client struct { // actual client
	K8sClientset        *kubernetes.Interface
	SecretManagerClient *secretmanager.Client
	Ctx                 context.Context
}

// structs for synchronzation pairs
type SyncPairCollection struct {
	Pairs []SyncPair
}
type SyncPair struct {
	Source      Target
	Destination Target
}
type Target struct {
	Kubernetes    *KubernetesSecret
	SecretManager *SecretManagerSecret
}
type KubernetesSecret struct {
	Namespace string
	Secret    string
	Version   string
	Data      map[string][]byte
}
type SecretManagerSecret struct {
	Project string
	Secret  string
	Version string
	Data    []uint8
}

// parse config to sync-pairs
func (config SecretSyncConfig) Parse() (collection SyncPairCollection) {
	for _, spec := range config.Specs {
		pair := SyncPair{
			Source:      createTarget(spec.Source),
			Destination: createTarget(spec.Destination),
		}
		collection.Pairs = append(collection.Pairs, pair)
	}
	return collection
}
func createTarget(ts TargetSpec) (target Target) {
	if k8s, gsm := ts.Kubernetes, ts.SecretManager; k8s != nil {
		target.Kubernetes = &KubernetesSecret{
			Namespace: k8s.Namespace,
			Secret:    k8s.Secret,
		}
	} else {
		target.SecretManager = &SecretManagerSecret{
			Project: gsm.Project,
			Secret:  gsm.Secret,
		}
	}
	return target
}

func (cl Client) UpdatedVersion(target Target) (bool, error) {
	updated := false
	if k8s, gsm := target.Kubernetes, target.SecretManager; k8s != nil {
		// k8s secret
		secret, err := (*cl.K8sClientset).CoreV1().Secrets(k8s.Namespace).Get(k8s.Secret, metav1.GetOptions{})
		if err != nil {
			return updated, err
		}

		newVersion := secret.ObjectMeta.ResourceVersion
		if newVersion != target.Kubernetes.Version {
			updated = true
		}

		// update to latest version
		target.Kubernetes.Version = newVersion
		target.Kubernetes.Data = secret.Data

		return updated, nil
	} else {
		// secret manager secret
		name := "projects/" + gsm.Project + "/secrets/" + gsm.Secret + "/versions/latest"
		getReq := &secretmanagerpb.GetSecretVersionRequest{
			Name: name,
		}
		getResult, err := cl.SecretManagerClient.GetSecretVersion(cl.Ctx, getReq)
		if err != nil {
			return updated, err
		}
		accReq := &secretmanagerpb.AccessSecretVersionRequest{
			Name: name,
		}
		accResult, err := cl.SecretManagerClient.AccessSecretVersion(cl.Ctx, accReq)
		if err != nil {
			return updated, err
		}

		versionSlice := strings.Split(getResult.Name, "/")
		newVersion := versionSlice[len(versionSlice)-1]
		if newVersion != target.SecretManager.Version {
			updated = true
		}

		// update to latest version
		target.SecretManager.Version = newVersion
		target.SecretManager.Data = accResult.Payload.Data

		return updated, nil
	}
}

func (target Target) PrintSecret() {
	if k8s, gsm := target.Kubernetes, target.SecretManager; k8s != nil {
		fmt.Printf("%s :\n %s\n", k8s.Version, k8s.Data)
	} else {
		fmt.Printf("%s :\n %s\n", gsm.Version, gsm.Data)

	}
}
