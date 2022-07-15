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
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
	"k8s.io/test-infra/experiment/clustersecretbackup/secretmanager"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/logrusutil"
)

const (
	secretID  = "prowjob-webhook-ca-cert"
	caCert = "ca-cert"
	caPrivKey = "ca-priv-key"
	caBundle = "ca-bundle"
	gcpSecretError = "failed to access secret version"
)

type ClientInterface interface {
	CreateSecret(ctx context.Context, secretID string) (*secretmanagerpb.Secret, error)
	AddSecretVersion(ctx context.Context, secretName string, payload []byte) error
	GetSecretValue(ctx context.Context, secretName, versionName string) ([]byte, error)
}

func main() {
	logrusutil.ComponentInit()
	logrus.SetLevel(logrus.DebugLevel)
	p := flag.String("project-id", 
	"", "Project ID for storing GCP Secrets")
	e := flag.Int("expiry-years", 30, "CA certificate expiry in years")
	dns := flagutil.NewStrings("validation-webhook-service", "validation-webhook-service.default", "validation-webhook-service.default.svc")
	flag.Var(&dns, "dns", "DNS Names CA-Cert config")
	flag.Parse()
	exp := *e
	projectId := *p
	if len(projectId) == 0 {
		logrus.Fatal("project-id flag not supplied")
	}
	client, err := secretmanager.NewClient(projectId, false)
	if err != nil {
		logrus.WithError(err).Fatal("Unable to create secret manager client")
	}
	ctx := context.Background()
	cert, privKey, err := getGCPSecrets(client, ctx, exp, dns)
	if err != nil && strings.Contains(err.Error(), gcpSecretError) {
		logrus.Infof("%v, Will now proceed to create GCP Secret", err)
		cert, privKey, err = createGCPSecret(client, ctx, exp, dns.Strings())
		if err != nil {
			logrus.WithError(err).Fatal("Unable to create ca certificate")
		}
	} else if err != nil {
		logrus.WithError(err).Fatal("Unable to get GCP Secret")
	}
	if err = isCertValid(cert); err != nil {
		logrus.WithError(err).Info("Certificate is not valid, will replace.")
		cert, privKey, err = updateGCPSecret(client, ctx, exp, dns.Strings())
		if err != nil {
			logrus.WithError(err).Fatal("Unable to update GCP Secret")
		}
	} 
	tempDir, err := ioutil.TempDir("", "cert")
	if err != nil {
		logrus.WithError(err).Fatal("Unable to create temp directory")
	}
	defer os.RemoveAll(tempDir)
	certFile := filepath.Join(tempDir, "certFile.pem")
	if err := ioutil.WriteFile(certFile, []byte(cert), 0666); err != nil {
		logrus.WithError(err).Fatal("Could not write contents of cert file")
	}
	privKeyFile := filepath.Join(tempDir, "privKey.pem")
	if err := ioutil.WriteFile(privKeyFile, []byte(privKey), 0666); err != nil {
		logrus.WithError(err).Fatal("Could not write contents of privKey file")
	}
	http.HandleFunc("/validate", serveValidate)
	logrus.Info("Listening on port 8008...")
	logrus.Fatal(http.ListenAndServeTLS(":8008", certFile, privKeyFile, nil))
}

func getGCPSecrets(client ClientInterface, ctx context.Context, expiry int, dns flagutil.Strings) (string, string, error) {
	secretsMap := make(map[string]string)
	data, err := client.GetSecretValue(ctx, secretID, "latest")
	if err != nil {
		return "", "", fmt.Errorf("%s, unable to get secret value: %v", gcpSecretError, err)
	}

	err = json.Unmarshal(data, &secretsMap)
	if err != nil {
		return "", "", fmt.Errorf("error unmarshalling CA cert secret data: %v", err)
	}

	cert := secretsMap[caCert]
	privKey := secretsMap[caPrivKey]

	return cert, privKey, nil
}


func createGCPSecret(client ClientInterface, ctx context.Context, expiry int, dns []string) (string, string, error) {
	if _, err := client.CreateSecret(ctx, secretID); err != nil {
		return "", "", fmt.Errorf("unable to create secret %v", err)
	}
	serverCertPerm, serverPrivKey, err := updateGCPSecret(client, ctx, expiry, dns)
	if err != nil {
		return "", "", fmt.Errorf("unable to write secret value %v", err)
	}
	return serverCertPerm, serverPrivKey, nil
}

func updateGCPSecret(client ClientInterface, ctx context.Context, expiry int, dns[] string) (string, string, error) {
	serverCertPerm, serverPrivKey, secretData, err := genSecretData(expiry, dns)
	if err != nil {
		return "", "", err
	}

	if err := client.AddSecretVersion(ctx, secretID, secretData); err != nil {
		return "", "", fmt.Errorf("unable to add secret version %v", err)
	}

	return serverCertPerm, serverPrivKey, nil
}

func genSecretData(expiry int, dns []string) (string, string, []byte, error) {
	serverCertPerm, serverPrivKey, caPem, err := genCert(expiry, dns)
	if err != nil {
		return "", "", nil, fmt.Errorf("could not generate ca credentials")
	}
	caSecrets := map[string]string{
	    caCert: serverCertPerm,
	    caPrivKey: serverPrivKey,
	    caBundle: caPem,
	}
	secretData, err := json.Marshal(caSecrets)

	if err != nil {
		return "", "", nil, fmt.Errorf("error marshalling CA cert secret data: %v", err)
	}

	return serverCertPerm, serverPrivKey, secretData, nil
}
