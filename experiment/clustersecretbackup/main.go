/*
Copyright 2021 The Kubernetes Authors.

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
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable all auth provider plugins
	"k8s.io/test-infra/experiment/clustersecretbackup/secretmanager"
	"k8s.io/test-infra/gencred/pkg/util"

	prowflagutil "k8s.io/test-infra/prow/flagutil"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// options are the available command-line flags.
type options struct {
	help       bool
	configPath string
	dryRun     bool
	kubernetes prowflagutil.KubernetesOptions
}

type secretsConfig struct {
	Secrets []secretConfig `yaml:"secrets"`
}

type secretConfig struct {
	Name        string `yaml:"name"`
	Namespace   string `yaml:"namespace"`
	Description string `yaml:"description"`
	SecretName  string `yaml:"secret_name"`
	// Project is where the secret is backed up at
	Project string `yaml:"project"`
}

// parseFlags parses the command-line flags.
func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.BoolVar(&o.help, "help", false, "")
	fs.StringVar(&o.configPath, "config-path", "", "Config file path defining which secrets to backup.")
	fs.BoolVar(&o.dryRun, "dryrun", false, "Controls whether this is dry run or not")
	o.kubernetes.AddFlags(fs)
	fs.Parse(args)

	return o
}

// validateFlags validates the command-line flags.
func (o *options) validateFlags() error {
	if len(o.configPath) == 0 {
		return errors.New("--config-path option is required")
	}

	return nil
}

func backupSecret(ctx context.Context, secretmanagerClient secretmanager.ClientInterface, secretID string, payload []byte) error {
	ss, err := secretmanagerClient.ListSecrets(ctx)
	if err != nil {
		return err
	}
	var found bool
	for _, s := range ss {
		if strings.HasSuffix(s.Name, fmt.Sprintf("/%s", secretID)) {
			found = true
		}
	}
	if !found {
		if _, err = secretmanagerClient.CreateSecret(ctx, secretID); err != nil {
			return err
		}
	}
	return secretmanagerClient.AddSecretVersion(ctx, secretID, payload)
}

// process merges secret into a new secret for write.
func processAll(ctx context.Context, ssc secretsConfig, kubeClient ctrlruntimeclient.Client, secretmanagerClients map[string]secretmanager.ClientInterface) error {
	var lastErr error

	for _, sc := range ssc.Secrets {
		secret := &corev1.Secret{}
		if err := kubeClient.Get(ctx, ctrlruntimeclient.ObjectKey{
			Namespace: sc.Namespace,
			Name:      sc.Name,
		}, secret); err != nil {
			logrus.WithError(err).Errorf("Failed getting secret %s", sc.Name)
			lastErr = err
		}

		body, err := yaml.Marshal(secret)
		if err != nil {
			logrus.WithError(err).Errorf("Failed marshal secret %s", sc.Name)
			lastErr = err
		}
		if err := backupSecret(ctx, secretmanagerClients[sc.Project], "", body); err != nil {
			logrus.WithError(err).Errorf("Failed backing up secret %s", sc.Name)
			lastErr = err
		}
	}

	return lastErr
}

func main() {
	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.validateFlags(); err != nil {
		util.PrintErrAndExit(err)
	}

	content, err := ioutil.ReadFile(o.configPath)
	if err != nil {
		logrus.WithError(err).Fatalf("Failed read config file %s: %v", o.configPath, err)
	}
	var ssc secretsConfig
	if err := yaml.Unmarshal(content, &ssc); err != nil {
		logrus.WithError(err).Fatalf("Failed unmarshalling secrets config: %v", err)
	}

	secretmanagerClients := map[string]secretmanager.ClientInterface{}
	for _, sc := range ssc.Secrets {
		if _, ok := secretmanagerClients[sc.Project]; ok {
			continue
		}
		smc, err := secretmanager.NewClient(sc.Project)
		if err != nil {
			logrus.WithError(err).Fatalf("Failed creating secret manager client: %v", err)
		}
		secretmanagerClients[sc.Project] = smc
	}

	ctx := context.Background()

	infrastructureClusterConfig, err := o.kubernetes.InfrastructureClusterConfig(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatalf("Error getting infrastructure cluster config: %v", err)
	}
	kubeClient, err := ctrlruntimeclient.New(infrastructureClusterConfig, ctrlruntimeclient.Options{})
	if err != nil {
		logrus.WithError(err).Fatalf("Error getting infrastructure cluster config: %v", err)
	}

	if err := processAll(ctx, ssc, kubeClient, secretmanagerClients); err != nil {
		logrus.WithError(err).Fatal(err)
	}
}
