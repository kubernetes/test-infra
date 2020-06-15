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

type SecretSyncSpec struct {
	// the target can be either a K8s secret or a SecretManager secret
	Source      Target `yaml:"source"`
	Destination Target `yaml:"destination"`
}

type Target struct {
	// assert that one of the two should be `nil`
	Kubernetes    *KubernetesSpec    `yaml:"kubernetes"`
	SecretManager *SecretManagerSpec `yaml:"secretManager"`
}

type KubernetesSpec struct {
	Namespace string `yaml:"namespace"`
	Secret    string `yaml:"secret"`
}

type SecretManagerSpec struct {
	Project string `yaml:"project"`
	Secret  string `yaml:"secret"`
}

func (target Target) GetLatestSecretVersion(k8s_clientset *kubernetes.Clientset, secretManager_ctx context.Context, secretManager_client *secretmanager.Client) (int, map[string][]byte) {
	if k8s, gsm := target.Kubernetes, target.SecretManager; k8s != nil {
		return k8s.LatestSecretVersion(k8s_clientset)
	} else {
		return gsm.LatestSecretVersion(secretManager_ctx, secretManager_client)
	}
}

func (k8s *KubernetesSpec) LatestSecretVersion(k8s_clientset *kubernetes.Clientset) (int, map[string][]byte) {
	secret, err := k8s_clientset.CoreV1().Secrets(k8s.Namespace).Get(context.TODO(), k8s.Secret, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		fmt.Println(err)
	}
	version, _ := strconv.Atoi(secret.ObjectMeta.ResourceVersion)

	return version, secret.Data
}

func (gsm *SecretManagerSpec) LatestSecretVersion(ctx context.Context, client *secretmanager.Client) (int, map[string][]byte) {

	name := "projects/" + gsm.Project + "/secrets/" + gsm.Secret + "/versions/latest"
	// Build the get request.
	get_req := &secretmanagerpb.GetSecretVersionRequest{
		Name: name,
	}

	// Call the API.
	get_result, _ := client.GetSecretVersion(ctx, get_req)

	version_slice := strings.Split(get_result.Name, "/")
	version, _ := strconv.Atoi(version_slice[len(version_slice)-1])

	// Build the access request.
	acc_req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}

	// Call the API.
	acc_result, _ := client.AccessSecretVersion(ctx, acc_req)
	buf := make(map[interface{}]interface{})
	yaml.Unmarshal(acc_result.Payload.Data, &buf)

	secret := make(map[string][]byte)

	for key, val := range buf {
		secret[fmt.Sprintf("%v", key)] = []byte(fmt.Sprintf("%v", val))
	}

	return version, secret
}
