package main

import (
	"context"
	// b64 "encoding/base64"
	// "bytes"
	"fmt"
	"gopkg.in/yaml.v2"
	"strconv"
	"strings"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type SecretSyncConfig struct {
	Specs []SecretSyncSpec `yaml:"specs"`
}

type SecretSyncSpec struct {
	// the target can be either a K8s secret or a SecretManager secret
	Source      Target `yaml:"source"`
	Destination Target `yaml:"destination"`
}

type Target struct {
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

func (target Target) GetLatestSecretVersion(k8sClientset *kubernetes.Clientset, secretManagerCtx context.Context, secretManagerClient *secretmanager.Client) (int, map[string][]byte) {
	if k8s, gsm := target.Kubernetes, target.SecretManager; k8s != nil {
		return k8s.LatestSecretVersion(k8sClientset)
	} else {
		return gsm.LatestSecretVersion(secretManagerCtx, secretManagerClient)
	}
}

func (k8s *KubernetesSpec) LatestSecretVersion(k8sClientset *kubernetes.Clientset) (int, map[string][]byte) {
	secret, err := k8sClientset.CoreV1().Secrets(k8s.Namespace).Get(k8s.Secret, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		fmt.Println(err)
	}
	version, _ := strconv.Atoi(secret.ObjectMeta.ResourceVersion)

	return version, secret.Data
}

func (gsm *SecretManagerSpec) LatestSecretVersion(ctx context.Context, client *secretmanager.Client) (int, map[string][]byte) {

	name := "projects/" + gsm.Project + "/secrets/" + gsm.Secret + "/versions/latest"
	// Build the get request.
	getReq := &secretmanagerpb.GetSecretVersionRequest{
		Name: name,
	}

	// Call the API.
	getResult, _ := client.GetSecretVersion(ctx, getReq)

	versionSlice := strings.Split(getResult.Name, "/")
	version, _ := strconv.Atoi(versionSlice[len(versionSlice)-1])

	// Build the access request.
	accReq := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}

	// Call the API.
	accResult, _ := client.AccessSecretVersion(ctx, accReq)
	buf := make(map[interface{}]interface{})
	yaml.Unmarshal(accResult.Payload.Data, &buf)

	secret := make(map[string][]byte)

	for key, val := range buf {
		secret[fmt.Sprintf("%v", key)] = []byte(fmt.Sprintf("%v", val))
	}

	return version, secret
}
