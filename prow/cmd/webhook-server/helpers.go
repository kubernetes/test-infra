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
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	admregistration "k8s.io/api/admissionregistration/v1"
	b64 "encoding/base64"
	"encoding/json"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"context"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
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
		return "", "", "", err
	}

	// Self signed CA certificate
	caBytes, err := x509.CreateCertificate(cryptorand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return "", "", "", err
	}

	// PEM encode CA cert
	caPEM = new(bytes.Buffer)
	err = pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})
	if err != nil {
		return "", "", "", err
	}

	commonName := "validation-webhook-service.default.svc"

	// server cert config
	cert := &x509.Certificate{
		DNSNames:     dnsNames,
		SerialNumber: big.NewInt(1658),  //unique identifier for cert
		Subject: pkix.Name{
			CommonName:   commonName,
			Organization: []string{org},
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(expiry, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},  //unique identifier for cert
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	// server private key
	serverPrivKey, err := rsa.GenerateKey(cryptorand.Reader, 4096)
	if err != nil {
		return "", "", "", err
	}

	// sign the server cert
	serverCertBytes, err := x509.CreateCertificate(cryptorand.Reader, cert, ca, &serverPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return "", "", "", err
	}

	// PEM encode the  server cert and key
	serverCertPEM = new(bytes.Buffer)
	err = pem.Encode(serverCertPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: serverCertBytes,
	})
	if err != nil {
		return "", "", "", err
	}

	serverPrivKeyPEM = new(bytes.Buffer)
	err = pem.Encode(serverPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(serverPrivKey),
	})
	if err != nil {
		return "", "", "", err
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
		err := fmt.Sprintf("certificated expired %v", time.Now().UTC().Sub(certificate.NotAfter))
		return fmt.Errorf(err)
	}
	return nil
}

func getGCPSecrets(client ClientInterface, ctx context.Context, expiry int, dns prowflagutil.Strings) (string, string, string, error) {
	secretsMap := make(map[string]string)
	data, err := client.GetSecretValue(ctx, secretID, "latest")
	if err != nil {
		return "", "", "", fmt.Errorf(gcpSecretError)
	}

	err = json.Unmarshal(data, &secretsMap)
	if err != nil {
		return "", "", "", fmt.Errorf("error marshalling CA cert secret data: %v", err)
	}

	cert := secretsMap[caCert]
	privKey := secretsMap[caPrivKey]
	caPem := secretsMap[caBundle]

	return cert, privKey, caPem, nil
}


func createGCPSecret(client ClientInterface, ctx context.Context, expiry int, dns []string) (string, string, string, error) {
	serverCertPerm, serverPrivKey, caPem, secretData, err := genSecretData(expiry, dns)
	if err != nil {
		return "", "", "", err
	}

	if _, err := client.CreateSecret(ctx, secretID); err != nil {
		return "", "", "", fmt.Errorf("unable to create secret %v", err)
	}

	if err := client.AddSecretVersion(ctx, secretID, secretData); err != nil {
		return "", "", "", fmt.Errorf("unable to add secret version %v", err)
	}

	return serverCertPerm, serverPrivKey, caPem, nil
}

func updateGCPSecret(client ClientInterface, ctx context.Context, expiry int, dns[] string) (string, string, string, error) {
	serverCertPerm, serverPrivKey, caPem, secretData, err := genSecretData(expiry, dns)
	if err != nil {
		return "", "", "", err
	}

	if err := client.AddSecretVersion(ctx, secretID, secretData); err != nil {
		return "", "", "", fmt.Errorf("unable to add secret version %v", err)
	}

	return serverCertPerm, serverPrivKey, caPem, nil
}

func genSecretData(expiry int, dns []string) (string, string, string, []byte, error) {
	serverCertPerm, serverPrivKey, caPem, err := genCert(expiry, dns)
	if err != nil {
		return "", "", "",  nil, fmt.Errorf("could not generate ca credentials")
	}
	caSecrets := map[string]string{
	    caCert: serverCertPerm,
	    caPrivKey: serverPrivKey,
	    caBundle: caPem,
	}
	secretData, err := json.Marshal(caSecrets)

	if err != nil {
		return "", "", "", nil, fmt.Errorf("error unmarshalling CA cert secret data: %v", err)
	}

	return serverCertPerm, serverPrivKey, caPem, secretData, nil
}

func createOrPatchValidationWebhookConfig(ctx context.Context, caPem string, client ctrlruntimeclient.Client, isPatch bool) error {
	operations  := []admregistration.OperationType{"CREATE", "UPDATE"}
	scope := admregistration.ScopeType("*")
	path := "/validate"
	sideEffects := admregistration.SideEffectClass("None")
	caPemEncoded := []byte(b64.StdEncoding.EncodeToString([]byte(caPem)))

	validatingWebhookConfig :=  &admregistration.ValidatingWebhookConfiguration {
		TypeMeta: v1.TypeMeta {
			Kind : "ValidatingWebhookConfiguration",
			APIVersion: "admissionregistration.k8s.io/v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name : "prow-job-validating-webhook-config.prow.k8s.io",
		},
		Webhooks: []admregistration.ValidatingWebhook{
			{
				Name: "prow-job-validating-webhook-config.prow.k8s.io",
				ObjectSelector: &v1.LabelSelector{
					MatchLabels: map[string]string {
						"admission-webhook": "enabled",
					},
				},
				Rules: []admregistration.RuleWithOperations{
					{
						Operations: operations,
						Rule: admregistration.Rule{
							APIGroups: []string{""},
							APIVersions: []string{"v1"},
							Resources: []string{"prowjobs"},
							Scope: &scope,
						},

					},

				},
				ClientConfig: admregistration.WebhookClientConfig{
					Service: &admregistration.ServiceReference{
						Namespace: "default",
						Name: "prowjob-validation-webhook",
						Path: &path,
					},
					CABundle: caPemEncoded,
				},
				SideEffects: &sideEffects,
				AdmissionReviewVersions: []string{"v1"},
			},

		},
	}
	
	createOptions := &ctrlruntimeclient.CreateOptions{
		FieldManager: "webhook-server",
	}
	
	if !isPatch {
		if err := client.Create(ctx, validatingWebhookConfig, createOptions); err != nil {
			return fmt.Errorf("failed to create validation webhook config: %w", err)
		}
	} else {
		if err := client.Patch(ctx, validatingWebhookConfig, ctrlruntimeclient.MergeFrom(validatingWebhookConfig)); err != nil {
			return fmt.Errorf("failed to patch validation webhook config: %w", err)
		}
	}
	
	return nil
}

