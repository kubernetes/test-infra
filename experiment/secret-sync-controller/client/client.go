package client

// package client implemets client creation utilities
// creates K8s clientset and Secret Manager client

import (
	"context"
	"os"
	"path/filepath"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	prowkube "k8s.io/test-infra/prow/kube"
)

func NewK8sClientset() (*kubernetes.Interface, error) {
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	// create the clientset
	clientset, err := prowkube.GetKubernetesClient("", kubeconfig)
	if err != nil {
		return nil, err
	}
	return &clientset, nil
}

func NewSecretManagerClient(ctx context.Context) (*secretmanager.Client, error) {
	// Create the client.
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return client, nil
}
