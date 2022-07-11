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
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	
	"github.com/sirupsen/logrus"
	secretmanagerpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
	"k8s.io/test-infra/experiment/clustersecretbackup/secretmanager"
	"k8s.io/test-infra/pkg/flagutil"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/logrusutil"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
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

type options struct {
	projectId string
	expiryInYears int
	dnsNames []string
	dryRun                 bool
	kubernetes             prowflagutil.KubernetesOptions
}

func (o *options) Validate() error {
	optionGroup := []flagutil.OptionGroup{&o.kubernetes}
	if err := optionGroup[0].Validate(o.dryRun); err != nil {
		return err
	}
	if len(o.projectId) == 0 || o.expiryInYears < 0 {
		return fmt.Errorf("project id not supplied")
	}
	return nil
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options	
	fs.StringVar(&o.projectId, "project-id", "colew-test", "Project ID for storing GCP Secrets")
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
	client, err := secretmanager.NewClient(o.projectId, false)
	if err != nil {
		logrus.WithError(err).Fatal("Unable to create secret manager client")
	}
	ctx := context.Background()
	kubeCfg, err := o.kubernetes.InfrastructureClusterConfig(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kubeconfig.")
	}
	cl, err := ctrlruntimeclient.New(kubeCfg, ctrlruntimeclient.Options{})
	if err != nil {
		logrus.WithError(err).Fatal("Could not create writer client")
	}
	cert, privKey, _, err := getGCPSecrets(client, ctx, o.expiryInYears, dns)
	var caPem string
	var secretUnavailable bool
	if err != nil {
		secretUnavailable = strings.Contains(err.Error(), gcpSecretError)
	}
	if err != nil && secretUnavailable {
		logrus.Info(err)
		cert, privKey, caPem, err = createGCPSecret(client, ctx, o.expiryInYears, dns.Strings())
		if err != nil {
			logrus.WithError(err).Fatal("Unable to create ca certificate")
		}
		err := createOrPatchValidationWebhookConfig(ctx, caPem, cl, false)
		if err != nil {
			logrus.WithError(err).Fatal("Unable to generate ValidationWebhookConfig")
		}
	} else if err != nil && !secretUnavailable {
		logrus.WithError(err).Fatal("Unable to get GCP Secret")
	}
	if err = isCertValid(cert); err != nil {
		logrus.Info(err)
		cert, privKey, caPem, err = updateGCPSecret(client, ctx, o.expiryInYears, dns.Strings())
		if err != nil {
			logrus.WithError(err).Fatal("Unable to update GCP Secret")
		}
		err := createOrPatchValidationWebhookConfig(ctx, caPem, cl, true)
		if err != nil {
			logrus.WithError(err).Fatal("Unable to generate ValidationWebhookConfig")
		}
	} 
	http.HandleFunc("/validate", serveValidate)
	logrus.Info("Listening on port 8008...")
	logrus.Fatal(http.ListenAndServeTLS(":8008", cert, privKey, nil))
}


