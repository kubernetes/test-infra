package main

// configuration and sync-pair defination

import (
	"context"
	"encoding/base64"
	"fmt"
	"gopkg.in/yaml.v2"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

// Structs for configuration
type SecretSyncConfig struct {
	Specs []SecretSyncSpec `yaml:"specs"`
}

type SecretSyncSpec struct {
	// the Source is always a SecretManager secret
	// the Destination is always a Kubernetes secret
	Source      SecretManagerSpec `yaml:"source"`
	Destination KubernetesSpec    `yaml:"destination"`
}

type KubernetesSpec struct {
	Namespace string `yaml:"namespace"`
	Secret    string `yaml:"secret"`
	Key       string `yaml:"key"`
}

type SecretManagerSpec struct {
	Project string `yaml:"project"`
	Secret  string `yaml:"secret"`
}

func (config SecretSyncConfig) String() string {
	d, _ := yaml.Marshal(config)
	return string(d)
}

type SecretData struct {
	Data []byte
	// Potentially more information
}

// structs for client interface
type ClientInterface interface {
	GetKubernetesSecret(KubernetesSpec) (*SecretData, error)
	GetSecretManagerSecret(SecretManagerSpec) (*SecretData, error)
	UpsertKubernetesSecret(KubernetesSpec, *SecretData) (*SecretData, error)
}
type Client struct { // actual client
	K8sClientset        *kubernetes.Interface
	SecretManagerClient *secretmanager.Client
}

// GetKubernetesSecret gets the K8s secret data specified in KubernetesSpec.
// It gets a single secret values with the given spec.Key.
func (cl Client) GetKubernetesSecret(spec KubernetesSpec) (*SecretData, error) {
	secret, err := (*cl.K8sClientset).CoreV1().Secrets(spec.Namespace).Get(spec.Secret, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// Get the secert value according to the key.
	// Allow the case where the key-value pair for spec.Key is not yet create,
	// a key-value pair will be inserted with the UpsertKubernetesSecret() function.
	value, ok := secret.Data[spec.Key]
	if !ok {
		return new(SecretData), nil
	}

	return &SecretData{value}, nil
}

// GetKubernetesSecret gets the SecretManager secret data specified in SecretManagerSpec
func (cl Client) GetSecretManagerSecret(spec SecretManagerSpec) (*SecretData, error) {
	ctx := context.TODO()
	name := "projects/" + spec.Project + "/secrets/" + spec.Secret + "/versions/latest"

	accReq := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}
	accResult, err := cl.SecretManagerClient.AccessSecretVersion(ctx, accReq)
	if err != nil {
		return nil, err
	}

	return &SecretData{accResult.Payload.Data}, nil
}

// UpsertKubernetesSecret updates or inserts a key-value pair in the K8s secret with the given spec.Key.
func (cl Client) UpsertKubernetesSecret(spec KubernetesSpec, src *SecretData) (*SecretData, error) {
	// encode with base64 encoding
	encodedSrc := base64.StdEncoding.EncodeToString(src.Data)
	patch := []byte("{\"data\":{\"" + spec.Key + "\": \"" + encodedSrc + "\"}}")
	secret, err := (*cl.K8sClientset).CoreV1().Secrets(spec.Namespace).Patch(spec.Secret, types.MergePatchType, patch)
	if err != nil {
		return nil, err
	}

	// get the secert value according to the key
	value, ok := secret.Data[spec.Key]
	if !ok {
		return nil, fmt.Errorf("K8s:/namespaces/%s/secrets/%s does not contain key: %s", spec.Namespace, spec.Secret, spec.Key)
	}

	return &SecretData{value}, nil
}
