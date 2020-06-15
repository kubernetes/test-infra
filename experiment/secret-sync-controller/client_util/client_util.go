package client_util

import (
	"context"
	"flag"
	"os"
	"path/filepath"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"

	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
)

func NewK8sClientset() *kubernetes.Clientset {
	var kubeconfig *string
	if home := os.Getenv("HOME"); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}
	return clientset
}

func NewSecretManagerClient(ctx context.Context) *secretmanager.Client {
	// Create the client.
	client, err := secretmanager.NewClient(ctx)
	if err != nil {
		panic(err.Error())
	}
	return client
}
