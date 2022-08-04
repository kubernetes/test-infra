/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"context"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"io/ioutil"

	"encoding/json"
	"encoding/pem"

	"fmt"

	"math/big"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	admregistration "k8s.io/api/admissionregistration/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/plank"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const org = "prow.k8s.io"

func genCert(expiry int, dnsNames []string) (string, string, string, error) {
	//https://gist.github.com/velotiotech/2e0cfd15043513d253cad7c9126d2026#file-initcontainer_main-go
	var caPEM, serverCertPEM, serverPrivKeyPEM *bytes.Buffer
	// CA config
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2020), //unique identifier for cert
		Subject: pkix.Name{
			Organization: []string{org},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(expiry, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// CA private key
	caPrivKey, err := rsa.GenerateKey(cryptorand.Reader, 4096)
	if err != nil {
		return "", "", "", fmt.Errorf("error generating ca private key: %v", err)
	}

	// Self signed CA certificate
	caBytes, err := x509.CreateCertificate(cryptorand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("error generating signed ca certificate: %v", err)
	}

	// PEM encode CA cert
	caPEM = new(bytes.Buffer)
	err = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	if err != nil {
		return "", "", "", fmt.Errorf("error encoding ca certificate: %v", err)
	}

	// server cert config
	cert := &x509.Certificate{
		DNSNames:     dnsNames,
		SerialNumber: big.NewInt(1658), //unique identifier for cert
		Subject: pkix.Name{
			CommonName:   "validation-webhook-service.default.svc",
			Organization: []string{org},
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(expiry, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6}, //unique identifier for cert
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	// server private key
	serverPrivKey, err := rsa.GenerateKey(cryptorand.Reader, 4096)
	if err != nil {
		return "", "", "", fmt.Errorf("error generating server private key: %v", err)
	}

	// sign the server cert
	serverCertBytes, err := x509.CreateCertificate(cryptorand.Reader, cert, ca, &serverPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return "", "", "", fmt.Errorf("error generating signed server certificate: %v", err)
	}

	// PEM encode the  server cert and key
	serverCertPEM = new(bytes.Buffer)
	err = pem.Encode(serverCertPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serverCertBytes,
	})
	if err != nil {
		return "", "", "", fmt.Errorf("error encoding server certificate: %v", err)
	}

	serverPrivKeyPEM = new(bytes.Buffer)
	err = pem.Encode(serverPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(serverPrivKey),
	})
	if err != nil {
		return "", "", "", fmt.Errorf("error encoding server private key: %v", err)
	}

	return serverCertPEM.String(), serverPrivKeyPEM.String(), caPEM.String(), nil

}

func isCertValid(cert string) error {
	block, _ := pem.Decode([]byte(cert))
	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}
	if time.Now().After(certificate.NotAfter) {
		return fmt.Errorf("certificated expired at %v", certificate.NotAfter)
	}
	return nil
}

func createSecret(client ClientInterface, ctx context.Context, clientoptions clientOptions) (string, string, string, error) {
	if err := client.CreateSecret(ctx, clientoptions.secretID); err != nil {
		return "", "", "", fmt.Errorf("unable to create secret %v", err)
	}

	serverCertPerm, serverPrivKey, caPem, err := updateSecret(client, ctx, clientoptions)
	if err != nil {
		return "", "", "", fmt.Errorf("unable to write secret value %v", err)
	}
	return serverCertPerm, serverPrivKey, caPem, nil
}

func updateSecret(client ClientInterface, ctx context.Context, clientoptions clientOptions) (string, string, string, error) {
	serverCertPerm, serverPrivKey, caPem, secretData, err := genSecretData(clientoptions.expiryInYears, clientoptions.dnsNames.Strings())
	if err != nil {
		return "", "", "", err
	}

	if err := client.AddSecretVersion(ctx, clientoptions.secretID, secretData); err != nil {
		return "", "", "", fmt.Errorf("unable to add secret version %v", err)
	}

	return serverCertPerm, serverPrivKey, caPem, nil
}

func genSecretData(expiry int, dns []string) (string, string, string, []byte, error) {
	serverCertPerm, serverPrivKey, caPem, err := genCert(expiry, dns)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("could not generate ca credentials")
	}
	caSecrets := map[string]string{
		certFile:     serverCertPerm,
		privKeyFile:  serverPrivKey,
		caBundleFile: caPem,
	}
	secretData, err := json.Marshal(caSecrets)

	if err != nil {
		return "", "", "", nil, fmt.Errorf("error unmarshalling CA cert secret data: %v", err)
	}

	return serverCertPerm, serverPrivKey, caPem, secretData, nil
}

func createValidatingWebhookConfig(ctx context.Context, caPem string, client ctrlruntimeclient.Client) error {
	operations := []admregistration.OperationType{"CREATE", "UPDATE"}
	scope := admregistration.ScopeType("*")
	path := "/validate"
	sideEffects := admregistration.SideEffectClass("None")

	validatingWebhookConfig := &admregistration.ValidatingWebhookConfiguration{
		TypeMeta: v1.TypeMeta{
			Kind:       "ValidatingWebhookConfiguration",
			APIVersion: "admissionregistration.k8s.io/v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "prow-job-validating-webhook-config.prow.k8s.io",
		},
		Webhooks: []admregistration.ValidatingWebhook{
			{
				Name: "prow-job-validating-webhook-config.prow.k8s.io",
				ObjectSelector: &v1.LabelSelector{
					MatchLabels: map[string]string{
						"admission-webhook": "enabled",
					},
				},
				Rules: []admregistration.RuleWithOperations{
					{
						Operations: operations,
						Rule: admregistration.Rule{
							APIGroups:   []string{"prow.k8s.io"},
							APIVersions: []string{"v1"},
							Resources:   []string{"prowjobs"},
							Scope:       &scope,
						},
					},
				},
				ClientConfig: admregistration.WebhookClientConfig{
					Service: &admregistration.ServiceReference{
						Namespace: "default",
						Name:      "prowjob-validation-webhook",
						Path:      &path,
					},
					CABundle: []byte(caPem),
				},
				SideEffects:             &sideEffects,
				AdmissionReviewVersions: []string{"v1"},
			},
		},
	}

	createOptions := &ctrlruntimeclient.CreateOptions{
		FieldManager: "webhook-server",
	}

	err := client.Create(ctx, validatingWebhookConfig, createOptions)
	if err != nil && strings.Contains(err.Error(), configAlreadyExistsError) {
		logrus.Info("ValidatingWebhookConfiguration already exists, proceeding to patch")
		if err := patchValidatingWebhookConfig(ctx, caPem, client); err != nil {
			return fmt.Errorf("failed to patch validation webhook config: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to create validation webhook config: %w", err)
	}

	return nil
}

func patchValidatingWebhookConfig(ctx context.Context, caPem string, client ctrlruntimeclient.Client) error {
	key := types.NamespacedName{
		Namespace: "default",
		Name:      "prow-job-validating-webhook-config.prow.k8s.io",
	}

	patchOptions := &ctrlruntimeclient.PatchOptions{
		FieldManager: "webhook-server",
	}
	var validatingWebhookConfig admregistration.ValidatingWebhookConfiguration
	if err := client.Get(ctx, key, &validatingWebhookConfig); err != nil {
		return fmt.Errorf("failed to get validation webhook config: %w", err)
	}
	oldValidatingWebhook := validatingWebhookConfig.DeepCopy()
	validatingWebhookConfig.Webhooks[0].ClientConfig.CABundle = []byte(caPem)
	if err := client.Patch(ctx, &validatingWebhookConfig, ctrlruntimeclient.MergeFrom(oldValidatingWebhook), patchOptions); err != nil {
		return fmt.Errorf("failed to patch validation webhook config: %w", err)
	}
	return nil
}

// this method runs on a go routine as a periodic task to continuously update the clusters in the config
func (wa *webhookAgent) fetchClusters(d time.Duration, ctx context.Context, statuses *map[string]plank.ClusterStatus, configAgent *config.Agent) error {
	ticker := time.NewTicker(d)
	defer ticker.Stop()
	cfg := configAgent.Config()
	opener, err := io.NewOpener(context.Background(), wa.storage.GCSCredentialsFile, wa.storage.S3CredentialsFile)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if location := cfg.Plank.BuildClusterStatusFile; location != "" {
				reader, err := opener.Reader(context.Background(), location)
				if err != nil {
					if !io.IsNotExist(err) {
						return fmt.Errorf("error opening build cluster status file for reading: %w", err)
					}
					logrus.Warnf("Build cluster status file location was specified, but could not be found: %v. This is expected when the location is first configured, before plank creates the file.", err)
				} else {
					defer reader.Close()
					b, err := ioutil.ReadAll(reader)
					if err != nil {
						return fmt.Errorf("error reading build cluster status file: %w", err)
					}
					var tempMap map[string]plank.ClusterStatus
					if err := json.Unmarshal(b, &tempMap); err != nil {
						return fmt.Errorf("error unmarshaling build cluster status file: %w", err)
					}
					wa.mu.Lock()
					wa.statuses = tempMap
					wa.mu.Unlock()
				}
			}
		}
	}
}
