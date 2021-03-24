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
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable all auth provider plugins
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/test-infra/experiment/clustersecretbackup/secretmanager"
	"k8s.io/test-infra/gencred/pkg/util"

	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type arrayFlags []string

func (af *arrayFlags) String() string {
	return strings.Join(*af, ",")
}

func (af *arrayFlags) Set(value string) error {
	*af = append(*af, value)
	return nil
}

// options are the available command-line flags.
type options struct {
	project    string
	cluster    string
	namespaces arrayFlags
	update     bool
	dryRun     bool
}

type client struct {
	kubeClient          ctrlruntimeclient.Client
	secretmanagerClient secretmanager.ClientInterface
	allSi               *corev1.SecretList
	options
}

type secretInfo struct {
	// Project is where the secret is backed up at
	project       string
	cluster       string
	clusterSecret *corev1.Secret
}

func (si *secretInfo) gsmSecretName() string {
	// Use cluster name, namespace and secret name is almost unique identifier.
	// However, if consider GCP allow creating clusters with the same name under
	// different zones, probably will need to add zones to this. Will address if
	// ever needed.
	return fmt.Sprintf("%s__%s__%s", si.cluster, si.clusterSecret.Namespace, si.clusterSecret.Name)
}

// gatherOptions parses the command-line flags.
func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.StringVar(&o.project, "project", "", "GCP project used for backing up secrets")
	fs.StringVar(&o.cluster, "cluster", "", "cluster context name used for backing up secrets")
	fs.Var(&o.namespaces, "namespace", "namespace to backup, can be passed in repeatedly")
	fs.BoolVar(&o.update, "update", false, "Controls whether update existing secret or not, if false then secret will only be created")
	fs.BoolVar(&o.dryRun, "dryrun", false, "Controls whether this is dry run or not")
	fs.Parse(args)

	return o
}

// validateFlags validates the command-line flags.
func (o *options) validateFlags() error {
	if len(o.project) == 0 {
		return errors.New("--project must be provided")
	}
	if len(o.cluster) == 0 {
		return errors.New("--cluster must be provided")
	}
	return nil
}

func newClient(o options) (*client, error) {
	secretmanagerClient, err := secretmanager.NewClient(o.project)
	if err != nil {
		return nil, fmt.Errorf("failed creating secret manager client: %w", err)
	}

	kubeClient, err := newKubeClients("", o.cluster)
	if err != nil {
		return nil, fmt.Errorf("failed creating kube client: %w", err)
	}
	return &client{
		kubeClient:          kubeClient,
		secretmanagerClient: secretmanagerClient,
		options:             o,
	}, nil
}

func newKubeClients(configPath, clusterName string) (ctrlruntimeclient.Client, error) {
	var loader clientcmd.ClientConfigLoader
	if configPath != "" {
		loader = &clientcmd.ClientConfigLoadingRules{ExplicitPath: configPath}
	} else {
		loader = clientcmd.NewDefaultClientConfigLoadingRules()
	}

	overrides := clientcmd.ConfigOverrides{}
	// Override the cluster name if provided.
	if clusterName != "" {
		overrides.Context.Cluster = clusterName
		overrides.CurrentContext = clusterName
	}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loader, &overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed create rest config: %v", err)
	}
	return ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{})
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
func (c *client) updateSingleSecret(ctx context.Context, si *secretInfo) error {
	secretID := si.gsmSecretName()
	payload, err := yaml.Marshal(si.clusterSecret)
	if err != nil {
		return fmt.Errorf("failed marshal secret %s: %w", si.clusterSecret.Name, err)
	}
	log := logrus.WithFields(logrus.Fields{
		"cluster":     si.cluster,
		"namespace":   si.clusterSecret.Namespace,
		"secret-name": si.clusterSecret.Name,
		"gsm-secret":  si.gsmSecretName(),
	})
	log.Info("Processing secret")
	if sat := "kubernetes.io/service-account-token"; string(si.clusterSecret.Type) == sat {
		log.Infof("Skipping: the secret type is %s", sat)
		return nil
	}
	if c.dryRun {
		log.Info("[Dryrun]: backing up secret")
		return nil
	}

	ss, err := c.secretmanagerClient.ListSecrets(ctx)
	if err != nil {
		return err
	}
	var found bool
	for _, s := range ss {
		if strings.HasSuffix(s.Name, fmt.Sprintf("/%s", secretID)) {
			found = true
		}
	}
	if found && !c.update {
		log.Info("Skipping: the secret already exist and --update is not true")
		return nil
	}
	// Now create or update
	if !found {
		log.Info("Creating secret in GSM")
		if _, err = c.secretmanagerClient.CreateSecret(ctx, secretID); err != nil {
			return err
		}
	}
	log.Info("Create secret version in GSM")
	return c.secretmanagerClient.AddSecretVersion(ctx, secretID, payload)
}

func (c *client) updateAllSecrets(ctx context.Context) error {
	for _, secret := range c.allSi.Items {
		si := &secretInfo{
			project:       c.project,
			cluster:       c.cluster,
			clusterSecret: &secret,
		}
		if err := c.updateSingleSecret(ctx, si); err != nil {
			return err
		}
	}
	return nil
}

func (c *client) loadClusterSecrets(ctx context.Context) error {
	c.allSi = &corev1.SecretList{}
	var listOptions []ctrlruntimeclient.ListOption
	for _, ns := range c.namespaces {
		listOptions = append(listOptions, &ctrlruntimeclient.ListOptions{
			Namespace: ns,
		})
	}
	return c.kubeClient.List(ctx, c.allSi, listOptions...)
}

func main() {
	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.validateFlags(); err != nil {
		util.PrintErrAndExit(err)
	}

	ctx := context.Background()

	c, err := newClient(o)
	if err != nil {
		logrus.WithError(err).Fatal("Failed creating client")
	}

	if err := c.loadClusterSecrets(ctx); err != nil {
		logrus.WithError(err).Fatal("Failed listing secrets")
	}

	if err := c.updateAllSecrets(ctx); err != nil {
		logrus.WithError(err).Fatal("Failed updating secrets")
	}
}
