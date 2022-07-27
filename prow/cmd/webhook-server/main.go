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

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/experiment/clustersecretbackup/secretmanager"
	"k8s.io/test-infra/pkg/flagutil"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/logrusutil"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	configAlreadyExistsError = "already exists"
	certFile                 = "certFile.pem"
	privKeyFile              = "privKeyFile.pem"
	caBundleFile             = "caBundle.pem"
)

type ClientInterface interface {
	CreateSecret(ctx context.Context, secretID string) error
	AddSecretVersion(ctx context.Context, secretName string, payload []byte) error
	GetSecretValue(ctx context.Context, secretName string, versionName string) ([]byte, bool, error)
}

type options struct {
	projectId      string
	expiryInYears  int
	dnsNames       []string
	dryRun         bool
	kubernetes     prowflagutil.KubernetesOptions
	fileSystemPath string
	secretID       string
}

var secretID string

func (o *options) Validate() error {
	optionGroup := []flagutil.OptionGroup{&o.kubernetes}
	if err := optionGroup[0].Validate(o.dryRun); err != nil {
		return err
	}
	if o.expiryInYears < 0 {
		return fmt.Errorf("invalid expiry years")
	}
	if o.projectId == "" && o.fileSystemPath == "" {
		return fmt.Errorf("both projectid and filesystem path cannot be specified")
	}
	if o.projectId != "" && o.fileSystemPath != "" {
		return fmt.Errorf("either projectid or filesystem path must be specified")
	}
	if o.projectId != "" && o.secretID == "" {
		return fmt.Errorf("secretID must be specified if choosing to use a GCP project")
	}
	return nil
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.StringVar(&o.projectId, "project-id", "", "Project ID for storing GCP Secrets")
	fs.StringVar(&o.fileSystemPath, "filesys-path", "./hello", "File system path for storing ca-cert secrets")
	fs.StringVar(&o.secretID, "secret-id", "", "GCP Project secret name")
	fs.IntVar(&o.expiryInYears, "expiry-years", 30, "CA certificate expiry in years")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether to mutate any real-world state")
	optionGroup := []flagutil.OptionGroup{&o.kubernetes}
	optionGroup[0].AddFlags(fs)
	fs.Parse(args)
	return o
}

func main() {
	logrusutil.ComponentInit()
	logrus.SetLevel(logrus.DebugLevel)
	dns := prowflagutil.NewStrings("validation-webhook-service", "validation-webhook-service.default", "validation-webhook-service.default.svc")
	flag.Var(&dns, "dns", "DNS Names CA-Cert config")
	flag.Parse()
	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	o.dnsNames = dns.Strings()
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}
	secretID = o.secretID
	kubeCfg, err := o.kubernetes.InfrastructureClusterConfig(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kubeconfig")
	}
	var certFile string
	var privKeyFile string
	ctx := context.Background()
	cl, err := ctrlruntimeclient.New(kubeCfg, ctrlruntimeclient.Options{})
	if err != nil {
		logrus.WithError(err).Fatal("Could not create writer client")
	}
	var client ClientInterface
	if o.projectId != "" {
		secretManagerClient, err := secretmanager.NewClient(o.projectId, false)
		if err != nil {
			logrus.WithError(err).Fatal("Unable to create secretmanager client", err)
		}
		client = newGCPClient(secretManagerClient)
		if err != nil {
			logrus.WithError(err).Fatal("Unable to create secret manager client")
		}
	}
	if o.fileSystemPath != "" {
		absPath, err := filepath.Abs(o.fileSystemPath)
		if err != nil {
			logrus.WithError(err).Fatal("Unable to generate absolute file path")
		}
		client = NewLocalFSClient(absPath, o.expiryInYears, o.dnsNames)
	}
	certFile, privKeyFile, err = handleSecrets(client, ctx, o, cl)
	if err != nil {
		logrus.WithError(err).Fatal("could not get necessary ca secret files", err)
	}
	http.HandleFunc("/validate", serveValidate)
	logrus.Info("Listening on port 8008...")
	logrus.Fatal(http.ListenAndServeTLS(":8008", certFile, privKeyFile, nil))
}

//get or creates the necessary ca secret files and returns the ca-cert file name, priv-key file name and tempDir name
//for use by the http listenAndServe
func handleSecrets(client ClientInterface, ctx context.Context, o options, cl ctrlruntimeclient.Client) (string, string, error) {
	var cert string
	var privKey string
	var caPem string
	secretsMap := make(map[string]string)
	data, exist, err := client.GetSecretValue(ctx, secretID, "latest")
	if err != nil {
		return "", "", err
	}
	if !exist {
		logrus.WithError(err).Info("Secret does not exist, now creating")
		cert, privKey, caPem, err = createSecret(client, ctx, o.expiryInYears, o.dnsNames)
		if err != nil {
			return "", "", fmt.Errorf("unable to create ca certificate %v", err)
		}
		if err = createValidatingWebhookConfig(ctx, caPem, cl); err != nil {
			return "", "", fmt.Errorf("unable to generate ValidationWebhookConfig %v", err)
		}
	} else {
		err = json.Unmarshal(data, &secretsMap)
		if err != nil {
			return "", "", fmt.Errorf("error marshalling CA cert secret data: %v", err)
		}
		cert = secretsMap[certFile]
		privKey = secretsMap[privKeyFile]
	}

	if err := isCertValid(cert); err != nil {
		logrus.WithError(err).Info("Certificate is not valid, will replace.")
		cert, privKey, caPem, err = updateSecret(client, ctx, o.expiryInYears, o.dnsNames)
		if err != nil {
			return "", "", fmt.Errorf("unable to update secret %v", err)
		}
		if err := patchValidatingWebhookConfig(ctx, caPem, cl); err != nil {
			return "", "", fmt.Errorf("unable to generate ValidationWebhookConfig %v", err)
		}
	}

	tempDir, err := ioutil.TempDir("", "cert")
	if err != nil {
		return "", "", fmt.Errorf("unable to create temp directory %v", err)
	}
	certFile := filepath.Join(tempDir, certFile)
	if err := ioutil.WriteFile(certFile, []byte(cert), 0666); err != nil {
		return "", "", fmt.Errorf("could not write contents of cert file %v", err)
	}
	privKeyFile := filepath.Join(tempDir, privKeyFile)
	if err := ioutil.WriteFile(privKeyFile, []byte(privKey), 0666); err != nil {
		return "", "", fmt.Errorf("could not write contents of privKey file %v", err)
	}

	return certFile, privKeyFile, nil
}
