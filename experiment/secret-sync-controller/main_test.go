package main

import (
	"fmt"
	"reflect"
	"testing"
)

type clientMock struct { // mock client
	K8sSecret           map[string]map[string]map[string][]byte
	SecretManagerSecret map[string]map[string][]byte
}

func (cl clientMock) GetKubernetesSecret(spec KubernetesSpec) (*SecretData, error) {
	_, ok := cl.K8sSecret[spec.Namespace][spec.Secret]
	if !ok {
		return nil, fmt.Errorf("Secret not found: K8s:/namespaces/%s/secrets/%s", spec.Namespace, spec.Secret)
	}
	val, ok := cl.K8sSecret[spec.Namespace][spec.Secret][spec.Key]
	if !ok {
		return new(SecretData), nil
	}
	return &SecretData{val}, nil
}
func (cl clientMock) GetSecretManagerSecret(spec SecretManagerSpec) (*SecretData, error) {
	val, ok := cl.SecretManagerSecret[spec.Project][spec.Secret]
	if !ok {
		return nil, fmt.Errorf("Secret not found: SecretManager:/projects/%s/secrets/%s", spec.Project, spec.Secret)
	}
	return &SecretData{val}, nil
}

func (cl clientMock) UpsertKubernetesSecret(spec KubernetesSpec, data *SecretData) (*SecretData, error) {
	_, ok := cl.K8sSecret[spec.Namespace][spec.Secret]
	if !ok {
		return nil, fmt.Errorf("Secret not found: K8s:/namespaces/%s/secrets/%s", spec.Namespace, spec.Secret)
	}
	cl.K8sSecret[spec.Namespace][spec.Secret][spec.Key] = data.Data
	return data, nil
}

func TestSync(t *testing.T) {

	var tests = []struct {
		input     SecretSyncSpec
		want      []byte
		expectErr bool
	}{
		{
			input: SecretSyncSpec{
				Source: SecretManagerSpec{
					Project: "projA",
					Secret:  "secretA",
				},
				Destination: KubernetesSpec{
					Namespace: "ns1",
					Secret:    "secret1",
					Key:       "key1",
				},
			},
			want:      []byte("valueOfSecretA"),
			expectErr: false,
		},
		{
			input: SecretSyncSpec{
				Source: SecretManagerSpec{
					Project: "projB",
					Secret:  "secretB",
				},
				Destination: KubernetesSpec{
					Namespace: "ns2",
					Secret:    "secret2",
					Key:       "key2",
				},
			},
			want:      []byte("valueOfSecretB"),
			expectErr: false,
		},
		{
			input: SecretSyncSpec{
				Source: SecretManagerSpec{
					Project: "projA",
					Secret:  "secretA",
				},
				Destination: KubernetesSpec{
					Namespace: "ns2",
					Secret:    "secret2",
					Key:       "key3",
				},
			},
			want:      []byte("valueOfSecretA"),
			expectErr: false,
		},
		{
			input: SecretSyncSpec{
				Source: SecretManagerSpec{
					Project: "projA",
					Secret:  "secretS",
				},
				Destination: KubernetesSpec{
					Namespace: "ns1",
					Secret:    "secret1",
					Key:       "key1",
				},
			},
			want:      []byte{},
			expectErr: true,
		},
		{
			input: SecretSyncSpec{
				Source: SecretManagerSpec{
					Project: "projA",
					Secret:  "secretA",
				},
				Destination: KubernetesSpec{
					Namespace: "ns2",
					Secret:    "secret3",
					Key:       "key2",
				},
			},
			want:      []byte{},
			expectErr: true,
		},
	}
	for _, tt := range tests {
		testname := fmt.Sprintf("%s", tt.input)
		t.Run(testname, func(t *testing.T) {
			client := clientMock{
				K8sSecret: map[string]map[string]map[string][]byte{
					"ns1": map[string]map[string][]byte{
						"secret1": map[string][]byte{
							"key1": []byte("valueOfSecret1"),
						},
					},
					"ns2": map[string]map[string][]byte{
						"secret2": map[string][]byte{
							"key2": []byte("valueOfSecret2"),
						},
					},
				},
				SecretManagerSecret: map[string]map[string][]byte{
					"projA": map[string][]byte{
						"secretA": []byte("valueOfSecretA"),
					},
					"projB": map[string][]byte{
						"secretB": []byte("valueOfSecretB"),
					},
				},
			}
			err := Sync(client, tt.input)
			if tt.expectErr {
				if err == nil {
					t.Errorf("Expected error but got none.")
				}
			} else {
				if err != nil {
					t.Errorf("Expected none error but got \"Error: %s\".", err)
				}
			}
			if err != nil {
				return
			}
			secret, _ := client.GetKubernetesSecret(tt.input.Destination)
			if !reflect.DeepEqual(secret.Data, tt.want) {
				t.Errorf("Expected %s but got %s.", tt.want, secret.Data)
			}
		})
	}
}

/*
func TestGetKubernetesSecret(t *testing.T) {

	var tests = []struct {
		input KubernetesSpec
		want  []byte
		error bool
	}{
		{
			input: KubernetesSpec{
				Namespace: "ns1",
				Secret:    "secret1",
				Key:       "key1",
			},
			want:  []byte("valueOfSecret1"),
			error: false,
		},
		{
			input: KubernetesSpec{
				Namespace: "ns2",
				Secret:    "secret2",
				Key:       "key2",
			},
			want:  []byte("valueOfSecret2"),
			error: false,
		},
		{
			input: KubernetesSpec{
				Namespace: "ns1",
				Secret:    "secret2",
				Key:       "key1",
			},
			want:  []byte{},
			error: true,
		},
		{
			input: KubernetesSpec{
				Namespace: "ns1",
				Secret:    "secret1",
				Key:       "key0",
			},
			want:  []byte{},
			error: true,
		},
	}
	for _, tt := range tests {
		testname := fmt.Sprintf("%s", tt.input)
		t.Run(testname, func(t *testing.T) {
			secret, err := mockClient.GetKubernetesSecret(tt.input)
			if tt.error {
				if err == nil {
					t.Errorf("Expected error but got none.")
				}
			} else {
				if err != nil {
					t.Errorf("Expected none error but got \"Error: %s\".", err)
				}
			}
			if err == nil && !reflect.DeepEqual(secret.Data, tt.want) {
				t.Errorf("Expected %s but got %s.", tt.want, secret.Data)
			}
		})
	}
}

func TestGetSecretManagerSecret(t *testing.T) {

	var tests = []struct {
		input SecretManagerSpec
		want  []byte
		error bool
	}{
		{
			input: SecretManagerSpec{
				Project: "projA",
				Secret:  "secretA",
			},
			want:  []byte("valueOfSecretA"),
			error: false,
		},
		{
			input: SecretManagerSpec{
				Project: "projB",
				Secret:  "secretB",
			},
			want:  []byte("valueOfSecretB"),
			error: false,
		},
		{
			input: SecretManagerSpec{
				Project: "projB",
				Secret:  "secretA",
			},
			want:  []byte{},
			error: true,
		},
		{
			input: SecretManagerSpec{
				Project: "ns1",
				Secret:  "secret1",
			},
			want:  []byte{},
			error: true,
		},
	}
	for _, tt := range tests {
		testname := fmt.Sprintf("%s", tt.input)
		t.Run(testname, func(t *testing.T) {
			secret, err := mockClient.GetSecretManagerSecret(tt.input)
			if tt.error {
				if err == nil {
					t.Errorf("Expected error but got none.")
				}
			} else {
				if err != nil {
					t.Errorf("Expected none error but got \"Error: %s\".", err)
				}
			}
			if err == nil && !reflect.DeepEqual(secret.Data, tt.want) {
				t.Errorf("Expected %s but got %s.", tt.want, secret.Data)
			}
		})
	}
}
*/
