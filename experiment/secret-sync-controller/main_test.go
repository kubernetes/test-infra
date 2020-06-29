package main

import (
	"bytes"
	"context"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/test-infra/experiment/secret-sync-controller/client"
	"testing"
)

/*
#testing-setup

secretManager:
  project:
  - name: k8s-jkns-gke-soak
    secret:
    - name: gsm-password
      data: gsm-password-v1
    - name: gsm-token
      data: gsm-token-v1
kubernetes:
  namespace:
  - name: ns-a
    secret:
    - name: secret-a
      key:
      - name: key-a
        data: old-token
  - name: ns-b
    secret:
    - name: secret-a
  - name: ns-c

*/

var setup = client.Setup{
	SecretManager: client.SecretManagerSetup{
		Projects: []client.ProjectSetup{
			{
				Name: "k8s-jkns-gke-soak",
				Secrets: []client.GSMSecretSetup{
					{
						Name: "gsm-password",
						Data: "gsm-password-v1",
					},
					{
						Name: "gsm-token",
						Data: "gsm-token-v1",
					},
				},
			},
		},
	},
	Kubernetes: client.KubernetesSetup{
		Namespaces: []client.NamespaceSetup{
			{
				Name: "ns-a",
				Secrets: []client.K8sSecretSetup{
					{
						Name: "secret-a",
						Keys: []client.KeySetup{
							{
								Name: "key-a",
								Data: "old-token",
							},
						},
					},
				},
			},
			{
				Name: "ns-b",
				Secrets: []client.K8sSecretSetup{
					{
						Name: "secret-b",
					},
				},
			},
			{
				Name: "ns-c",
			},
		},
	},
}

func TestSync(t *testing.T) {

	var tests = []struct {
		config    SecretSyncConfig
		want      client.KubernetesSetup
		expectErr bool
	}{
		{
			// happy path
			// syncing from <existing gsm secret> to <existing k8s secret> with <existing key>.
			// should update the key-value pair
			config: SecretSyncConfig{
				Specs: []SecretSyncSpec{
					{
						Source: SecretManagerSpec{
							Project: "k8s-jkns-gke-soak",
							Secret:  "gsm-password",
						},
						Destination: KubernetesSpec{
							Namespace: "ns-a",
							Secret:    "secret-a",
							Key:       "key-a",
						},
					},
				},
			},

			want: client.KubernetesSetup{
				Namespaces: []client.NamespaceSetup{
					{
						Name: "ns-a",
						Secrets: []client.K8sSecretSetup{
							{
								Name: "secret-a",
								Keys: []client.KeySetup{
									{
										Name: "key-a",
										Data: "gsm-password-v1",
									},
								},
							},
						},
					},
					{
						Name: "ns-b",
						Secrets: []client.K8sSecretSetup{
							{
								Name: "secret-b",
							},
						},
					},
					{
						Name: "ns-c",
					},
				},
			},
			expectErr: false,
		},
		{
			// syncing from <existing gsm secret> to <existing k8s secret> with <non-existing key>.
			// should insert a new key-value pair
			config: SecretSyncConfig{
				Specs: []SecretSyncSpec{
					{
						Source: SecretManagerSpec{
							Project: "k8s-jkns-gke-soak",
							Secret:  "gsm-token",
						},
						Destination: KubernetesSpec{
							Namespace: "ns-b",
							Secret:    "secret-b",
							Key:       "missed",
						},
					},
				},
			},

			want: client.KubernetesSetup{
				Namespaces: []client.NamespaceSetup{
					{
						Name: "ns-a",
						Secrets: []client.K8sSecretSetup{
							{
								Name: "secret-a",
								Keys: []client.KeySetup{
									{
										Name: "key-a",
										Data: "old-token",
									},
								},
							},
						},
					},
					{
						Name: "ns-b",
						Secrets: []client.K8sSecretSetup{
							{
								Name: "secret-b",
								Keys: []client.KeySetup{
									{
										Name: "missed",
										Data: "gsm-token-v1",
									},
								},
							},
						},
					},
					{
						Name: "ns-c",
					},
				},
			},
			expectErr: false,
		},
		{
			// syncing from <existing gsm secret> to <non-existing k8s secret> in <existing k8s namespace>.
			// should insert a new secret in that namespace with that key-value pair
			config: SecretSyncConfig{
				Specs: []SecretSyncSpec{
					{
						Source: SecretManagerSpec{
							Project: "k8s-jkns-gke-soak",
							Secret:  "gsm-token",
						},
						Destination: KubernetesSpec{
							Namespace: "ns-c",
							Secret:    "missed",
							Key:       "missed",
						},
					},
				},
			},

			want: client.KubernetesSetup{
				Namespaces: []client.NamespaceSetup{
					{
						Name: "ns-a",
						Secrets: []client.K8sSecretSetup{
							{
								Name: "secret-a",
								Keys: []client.KeySetup{
									{
										Name: "key-a",
										Data: "old-token",
									},
								},
							},
						},
					},
					{
						Name: "ns-b",
						Secrets: []client.K8sSecretSetup{
							{
								Name: "secret-b",
							},
						},
					},
					{
						Name: "ns-c",
						Secrets: []client.K8sSecretSetup{
							{
								Name: "missed",
								Keys: []client.KeySetup{
									{
										Name: "missed",
										Data: "gsm-token-v1",
									},
								},
							},
						},
					},
				},
			},
			expectErr: false,
		},
		{
			// syncing from <existing gsm secret> to <k8s secret> in <non-existing k8s namespace>.
			// should return error
			config: SecretSyncConfig{
				Specs: []SecretSyncSpec{
					{
						Source: SecretManagerSpec{
							Project: "k8s-jkns-gke-soak",
							Secret:  "gsm-token",
						},
						Destination: KubernetesSpec{
							Namespace: "missed",
							Secret:    "secret-a",
							Key:       "key-a",
						},
					},
				},
			},

			want: client.KubernetesSetup{
				Namespaces: []client.NamespaceSetup{
					{
						Name: "ns-a",
						Secrets: []client.K8sSecretSetup{
							{
								Name: "secret-a",
								Keys: []client.KeySetup{
									{
										Name: "key-a",
										Data: "old-token",
									},
								},
							},
						},
					},
					{
						Name: "ns-b",
						Secrets: []client.K8sSecretSetup{
							{
								Name: "secret-b",
							},
						},
					},
					{
						Name: "ns-c",
					},
				},
			},
			expectErr: true,
		},
		{
			// syncing from <non-existing gsm secret>.
			// should return error
			config: SecretSyncConfig{
				Specs: []SecretSyncSpec{
					{
						Source: SecretManagerSpec{
							Project: "k8s-jkns-gke-soak",
							Secret:  "missed",
						},
						Destination: KubernetesSpec{
							Namespace: "ns-a",
							Secret:    "secret-a",
							Key:       "key-a",
						},
					},
				},
			},
			want: client.KubernetesSetup{
				Namespaces: []client.NamespaceSetup{
					{
						Name: "ns-a",
						Secrets: []client.K8sSecretSetup{
							{
								Name: "secret-a",
								Keys: []client.KeySetup{
									{
										Name: "key-a",
										Data: "old-token",
									},
								},
							},
						},
					},
					{
						Name: "ns-b",
						Secrets: []client.K8sSecretSetup{
							{
								Name: "secret-b",
							},
						},
					},
					{
						Name: "ns-c",
					},
				},
			},
			expectErr: true,
		},
	}
	for _, tt := range tests {
		testname := fmt.Sprintf("%s", tt.config)
		t.Run(testname, func(t *testing.T) {

			// prepare clients
			k8sClientset, err := client.NewK8sClientset()
			if err != nil {
				t.Errorf("New kubernetes client failed: %s", err)
			}
			secretManagerClient, err := client.NewSecretManagerClient(context.Background())
			if err != nil {
				t.Errorf("New Secret Manager client failed: %s", err)
			}
			clientInterface := &client.Client{
				K8sClientset:        *k8sClientset,
				SecretManagerClient: *secretManagerClient,
			}

			controller := &SecretSyncController{
				Client:  clientInterface,
				Config:  &tt.config,
				RunOnce: true,
			}

			err = controller.Client.SetupTesting(setup)
			if err != nil {
				t.Error(err)
			}

			err = controller.ValidateAccess()
			if err != nil {
				t.Error(err)
			}

			var stopChan <-chan struct{}
			controller.Start(stopChan)

			// validate result
			for _, namespace := range tt.want.Namespaces {
				// check if the namespace exists
				_, err := clientInterface.K8sClientset.CoreV1().Namespaces().Get(namespace.Name, metav1.GetOptions{})
				if err != nil {
					t.Error(err)
				}

				for _, secret := range namespace.Secrets {
					// check if the secret exists
					secretObj, err := clientInterface.K8sClientset.CoreV1().Secrets(namespace.Name).Get(secret.Name, metav1.GetOptions{})
					if err != nil {
						t.Error(err)
					}

					for _, key := range secret.Keys {
						// Get the secret value according to the key.
						value, ok := secretObj.Data[key.Name]
						if !ok {
							t.Errorf("keys \"%s\" not found for namespaces/%s/secrets/%s", key.Name, namespace.Name, secret.Name)
						}
						if !bytes.Equal(value, []byte(key.Data)) {
							t.Errorf("Fail to validate namespaces/%s/secrets/%s. Expected %s but got %s.", namespace.Name, secret.Name, key.Data, value)
						}

					}
				}
			}

			// clean up the entire namespaces
			err = controller.Client.CleanupTesting(setup, true)
			if err != nil {
				t.Error(err)
			}
		})
	}
}
