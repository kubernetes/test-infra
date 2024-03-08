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
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/cmd/webhook-server/secretmanager"
	"k8s.io/test-infra/prow/config"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/plank"
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
	kubernetes     prowflagutil.KubernetesOptions
	secretID       string
	projectId      string
	expiryInYears  int
	dnsNames       prowflagutil.Strings
	fileSystemPath string
	config         configflagutil.ConfigOptions
	storage        prowflagutil.StorageClientOptions
	time           int
	dryRun         bool
}

type clientOptions struct {
	secretID      string
	expiryInYears int
	dnsNames      prowflagutil.Strings
}

type webhookAgent struct {
	storage  prowflagutil.StorageClientOptions
	statuses map[string]plank.ClusterStatus
	mu       sync.Mutex
	plank    config.Plank
}

func (o *options) DefaultAndValidate() error {
	optionGroup := []flagutil.OptionGroup{&o.kubernetes, &o.config, &o.storage}
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
	if o.dnsNames.StringSet().Len() == 0 {
		o.dnsNames.Add(prowjobAdmissionServiceName + ".default.svc")
	}
	return nil
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.StringVar(&o.projectId, "project-id", "", "Project ID for storing GCP Secrets")
	fs.StringVar(&o.fileSystemPath, "filesys-path", "./prowjob-webhook-ca-cert", "File system path for storing ca-cert secrets")
	fs.StringVar(&o.secretID, "secret-id", "", "GCP Project secret name")
	fs.IntVar(&o.expiryInYears, "expiry-years", 30, "CA certificate expiry in years")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether to mutate any real-world state")
	fs.IntVar(&o.time, "time", 1, "duration in minutes to fetch build clusters")
	fs.Var(&o.dnsNames, "dns", "DNS Names CA-Cert config")
	optionGroups := []flagutil.OptionGroup{&o.kubernetes, &o.config}
	for _, optionGroup := range optionGroups {
		optionGroup.AddFlags(fs)
	}
	fs.Parse(args)
	return o
}

func main() {
	logrusutil.ComponentInit()
	logrus.SetLevel(logrus.DebugLevel)
	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.DefaultAndValidate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}
	defer interrupts.WaitForGracefulShutdown()
	health := pjutil.NewHealth()
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
	statuses := make(map[string]plank.ClusterStatus)
	clientoptions := &clientOptions{
		secretID:      o.secretID,
		dnsNames:      o.dnsNames,
		expiryInYears: o.expiryInYears,
	}
	if o.projectId != "" {
		secretManagerClient, err := secretmanager.NewClient(o.projectId, false)
		if err != nil {
			logrus.WithError(err).Fatal("Unable to create secretmanager client", err)
		}
		client = newGCPClient(secretManagerClient, o.secretID)
		if err != nil {
			logrus.WithError(err).Fatal("Unable to create secret manager client")
		}
	}
	if o.fileSystemPath != "" {
		absPath, err := filepath.Abs(o.fileSystemPath)
		if err != nil {
			logrus.WithError(err).Fatal("Unable to generate absolute file path")
		}
		client = NewLocalFSClient(absPath, o.expiryInYears, o.dnsNames.Strings())
	}
	certFile, privKeyFile, err = handleSecrets(client, ctx, *clientoptions, cl)
	if err != nil {
		logrus.WithError(err).Fatal("could not get necessary ca secret files", err)
	}
	configAgent, err := o.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("could not create config agent")
	}
	cfg := configAgent.Config()
	wa := &webhookAgent{
		storage:  o.storage,
		statuses: statuses,
		plank:    cfg.Plank,
	}
	interrupts.Run(func(ctx context.Context) {
		wa.fetchClusters(time.Duration(o.time*int(time.Minute)), ctx, &wa.statuses, configAgent)
	})

	mux := http.NewServeMux()
	mux.HandleFunc(validatePath, wa.serveValidate)
	mux.HandleFunc(mutatePath, wa.serveMutate)
	s := http.Server{
		Addr: ":8008",
		TLSConfig: &tls.Config{
			ClientAuth: tls.NoClientCert,
		},
		Handler: mux,
	}
	logrus.Info("Listening on port 8008...")
	interrupts.ListenAndServeTLS(&s, certFile, privKeyFile, 5*time.Second)
	health.ServeReady(func() bool {
		return true
	})
}

// get or creates the necessary ca secret files and returns the ca-cert file name, priv-key file name and tempDir name
// for use by the http listenAndServe
func handleSecrets(client ClientInterface, ctx context.Context, clientoptions clientOptions, cl ctrlruntimeclient.Client) (string, string, error) {
	var cert string
	var privKey string
	var caPem string
	secretsMap := make(map[string]string)
	data, exist, err := client.GetSecretValue(ctx, clientoptions.secretID, "latest")
	if err != nil {
		return "", "", err
	}
	if !exist {
		logrus.WithError(err).Info("Secret does not exist, now creating")
		cert, privKey, caPem, err = createSecret(client, ctx, clientoptions)
		if err != nil {
			return "", "", fmt.Errorf("unable to create ca certificate %v", err)
		}
	} else {
		err = json.Unmarshal(data, &secretsMap)
		if err != nil {
			return "", "", fmt.Errorf("error marshalling CA cert secret data: %v", err)
		}
		cert = secretsMap[certFile]
		privKey = secretsMap[privKeyFile]
		if err := isCertValid(cert); err != nil {
			logrus.WithError(err).Info("Certificate is not valid, will replace.")
			cert, privKey, caPem, err = updateSecret(client, ctx, clientoptions)
			if err != nil {
				return "", "", fmt.Errorf("unable to update secret %v", err)
			}
		}
	}
	if err = reconcileWebhooks(ctx, caPem, cl); err != nil {
		return "", "", err
	}
	tempDir, err := os.MkdirTemp("", "cert")
	if err != nil {
		return "", "", fmt.Errorf("unable to create temp directory %v", err)
	}
	certFile := filepath.Join(tempDir, certFile)
	if err := os.WriteFile(certFile, []byte(cert), 0666); err != nil {
		return "", "", fmt.Errorf("could not write contents of cert file %v", err)
	}
	privKeyFile := filepath.Join(tempDir, privKeyFile)
	if err := os.WriteFile(privKeyFile, []byte(privKey), 0666); err != nil {
		return "", "", fmt.Errorf("could not write contents of privKey file %v", err)
	}
	return certFile, privKeyFile, nil
}
