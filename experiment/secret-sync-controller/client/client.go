package client

// package client implemets client creation utilities
// creates K8s clientset and Secret Manager client

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	prowkube "k8s.io/test-infra/prow/kube"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"google.golang.org/api/iterator"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
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

// structs for client interface
type Interface interface {
	ValidateKubernetesNamespace(string) error
	GetSecretManagerSecret(string, string) ([]byte, error)
	GetKubernetesSecret(string, string, string) ([]byte, error)
	UpsertKubernetesSecret(string, string, string, []byte) error
	// For testing
	CleanupSecretManagerSecrets(string) error
	CleanupKubernetesSecrets(string) error
	SetupTesting(Setup) error
	CleanupTesting(Setup, bool) error
}
type Client struct { // actual client
	K8sClientset        kubernetes.Interface
	SecretManagerClient secretmanager.Client
}

// ValidateKubernetesNamespace checks if a K8s namespace exists
func (cl *Client) ValidateKubernetesNamespace(namespace string) error {
	_, err := cl.K8sClientset.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil {
		return err
	}
	return nil
}

// GetKubernetesSecret gets the K8s secret data specified in KubernetesSpec.
// It gets a single secret values with the given spec.Key.
func (cl *Client) GetKubernetesSecret(namespace string, secretID string, key string) ([]byte, error) {
	// check if namespace exists
	_, err := cl.K8sClientset.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	// return empty slice if secretID or key does not exist
	// return a nil instead of an error
	secret, err := cl.K8sClientset.CoreV1().Secrets(namespace).Get(secretID, metav1.GetOptions{})
	if err != nil {
		return nil, nil
	}

	// Get the secret value according to the key.
	value, ok := secret.Data[key]
	if !ok {
		return nil, nil
	}

	return value, nil
}

// GetKubernetesSecret gets the SecretManager secret data specified in SecretManagerSpec
func (cl *Client) GetSecretManagerSecret(project string, secretID string) ([]byte, error) {
	ctx := context.TODO()
	name := "projects/" + project + "/secrets/" + secretID + "/versions/latest"

	accReq := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}
	accResult, err := cl.SecretManagerClient.AccessSecretVersion(ctx, accReq)
	if err != nil {
		return nil, err
	}

	return accResult.Payload.Data, nil
}

// UpsertKubernetesSecret updates or inserts a key-value pair in the K8s secret with the given spec.Key.
func (cl *Client) UpsertKubernetesSecret(namespace string, secretID string, key string, src []byte) error {
	// check if the namespace exists
	_, err := cl.K8sClientset.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil {
		return err
	}

	// encode with base64 encoding
	encodedSrc := base64.StdEncoding.EncodeToString(src)
	patch := []byte("{\"data\":{\"" + key + "\": \"" + encodedSrc + "\"}}")
	_, err = cl.K8sClientset.CoreV1().Secrets(namespace).Patch(secretID, types.MergePatchType, patch)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return err
		}

		// create a new secret in the case that it does not already exist
		newSecret := &v1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretID,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				key: src,
			},
		}
		_, err = cl.K8sClientset.CoreV1().Secrets(namespace).Create(newSecret)
		if err != nil {
			return err
		}
	}

	return nil
}

// CleanupSecretManagerSecrets deletes all secrets in the given project.
// Should be called with caution. Only for testing purpose.
func (cl *Client) CleanupSecretManagerSecrets(project string) error {
	ctx := context.TODO()
	parent := "projects/" + project
	req := &secretmanagerpb.ListSecretsRequest{
		Parent: parent,
	}

	it := cl.SecretManagerClient.ListSecrets(ctx, req)
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}

		if err != nil {
			return err
		}

		// delete all found secrets
		name := resp.Name
		req := &secretmanagerpb.DeleteSecretRequest{
			Name: name,
		}
		if err := cl.SecretManagerClient.DeleteSecret(ctx, req); err != nil {
			return err
		}
	}
	return nil
}

// CleanupKubernetesSecrets deletes all secrets under the given namespace in the cluster.
// Should be called with caution. Only for testing purpose.
func (cl *Client) CleanupKubernetesSecrets(namespace string) error {
	// check if the namespace exists
	_, err := cl.K8sClientset.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		} else {
			return err
		}
	}
	secretList, err := cl.K8sClientset.CoreV1().Secrets(namespace).List(metav1.ListOptions{})

	if err != nil {
		return err
	}

	for _, secret := range secretList.Items {
		secretID := secret.ObjectMeta.Name
		if strings.HasPrefix(secretID, "default-token") {
			continue
		}
		// delete all secrets expect for default-token-*
		err := cl.K8sClientset.CoreV1().Secrets(namespace).Delete(secretID, &metav1.DeleteOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

// CleanupKubernetesNamespace deletes the given namespace in the cluster.
// Should be called with caution. Only for testing purpose.
func (cl *Client) CleanupKubernetesNamespace(namespace string) error {
	// check if the namespace exists
	_, err := cl.K8sClientset.CoreV1().Namespaces().Get(namespace, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		} else {
			return err
		}
	}
	err = cl.K8sClientset.CoreV1().Namespaces().Delete(namespace, &metav1.DeleteOptions{})
	if err != nil {
		return err
	}
	return nil
}

type Setup struct {
	SecretManager SecretManagerSetup `yaml:"secretManager"`
	Kubernetes    KubernetesSetup    `yaml:"kubernetes"`
}
type SecretManagerSetup struct {
	Projects []ProjectSetup `yaml:"project"`
}
type ProjectSetup struct {
	Name    string           `yaml:"name"`
	Secrets []GSMSecretSetup `yaml:"secret"`
}
type GSMSecretSetup struct {
	Name string `yaml:"name"`
	Data string `yaml:"data"`
}
type KubernetesSetup struct {
	Namespaces []NamespaceSetup `yaml:"namespace"`
}
type NamespaceSetup struct {
	Name    string           `yaml:"name"`
	Secrets []K8sSecretSetup `yaml:"secret,omitempty"`
}
type K8sSecretSetup struct {
	Name string     `yaml:"name"`
	Keys []KeySetup `yaml:"key,omitempty"`
}
type KeySetup struct {
	Name string `yaml:"name"`
	Data string `yaml:"data"`
}

func (cl *Client) SetupTesting(setup Setup) error {
	for _, project := range setup.SecretManager.Projects {
		parent := "projects/" + project.Name
		for _, secret := range project.Secrets {
			// Create secret
			req := &secretmanagerpb.CreateSecretRequest{
				Parent:   parent,
				SecretId: secret.Name,
				Secret: &secretmanagerpb.Secret{
					Replication: &secretmanagerpb.Replication{
						Replication: &secretmanagerpb.Replication_Automatic_{
							Automatic: &secretmanagerpb.Replication_Automatic{},
						},
					},
				},
			}
			_, err := cl.SecretManagerClient.CreateSecret(context.TODO(), req)
			if err != nil {
				return err
			}
			// Add secret version
			verReq := &secretmanagerpb.AddSecretVersionRequest{
				Parent: parent + "/secrets/" + secret.Name,
				Payload: &secretmanagerpb.SecretPayload{
					Data: []byte(secret.Data),
				},
			}
			_, err = cl.SecretManagerClient.AddSecretVersion(context.TODO(), verReq)
			if err != nil {
				return err
			}
		}
	}

	for _, namespace := range setup.Kubernetes.Namespaces {
		// check if the namespace exists
		_, err := cl.K8sClientset.CoreV1().Namespaces().Get(namespace.Name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				// this namespace already exists
				newNamespace := &v1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: namespace.Name,
					},
				}
				_, err = cl.K8sClientset.CoreV1().Namespaces().Create(newNamespace)
				if err != nil {
					return err
				}
			} else {
				return err
			}
		}

		for _, secret := range namespace.Secrets {
			newSecret := &v1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secret.Name,
					Namespace: namespace.Name,
				},
			}
			_, err := cl.K8sClientset.CoreV1().Secrets(namespace.Name).Create(newSecret)
			if err != nil {
				return err
			}

			for _, key := range secret.Keys {
				// encode with base64 encoding
				encodedSrc := base64.StdEncoding.EncodeToString([]byte(key.Data))
				patch := []byte("{\"data\":{\"" + key.Name + "\": \"" + encodedSrc + "\"}}")
				_, err = cl.K8sClientset.CoreV1().Secrets(namespace.Name).Patch(secret.Name, types.MergePatchType, patch)
				if err != nil {
					return err
				}

			}
		}
	}
	return nil
}

func (cl *Client) CleanupTesting(setup Setup, ns bool) error {
	for _, project := range setup.SecretManager.Projects {
		err := cl.CleanupSecretManagerSecrets(project.Name)
		if err != nil {
			return err
		}
	}

	for _, namespace := range setup.Kubernetes.Namespaces {
		err := cl.CleanupKubernetesSecrets(namespace.Name)
		if err != nil {
			return err
		}
		if ns {
			// clean up the entire namespace
			// only run after all tests are complete
			err := cl.CleanupKubernetesNamespace(namespace.Name)
			if err != nil {
				return err
			}

			// wait until the namespace deletion completes
			for {
				_, err := cl.K8sClientset.CoreV1().Namespaces().Get(namespace.Name, metav1.GetOptions{})
				if err != nil {
					if !apierrors.IsNotFound(err) {
						return err
					} else {
						break
					}
				}
				time.Sleep(1000 * time.Millisecond)
			}
		}
	}
	return nil
}
