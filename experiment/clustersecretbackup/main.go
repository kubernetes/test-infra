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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/sirupsen/logrus"
	corev1 "k8s.io/api/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable all auth provider plugins
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api"
	"k8s.io/test-infra/experiment/clustersecretbackup/secretmanager"
	"k8s.io/test-infra/gencred/pkg/util"

	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	defaultSecretLabels = map[string]string{
		"update_time": time.Now().Format("2006-01-02-15-04-05"),
		"type":        "prow_backup",
		"source":      "",
	}
)

// options are the available command-line flags.
type options struct {
	project        string
	clusterContext string
	namespaces     []string
	secrets        map[string]string
	update         bool
	dryRun         bool
}

type client struct {
	kubeClient          ctrlruntimeclient.Client
	secretmanagerClient secretmanager.ClientInterface
	allSi               *corev1.SecretList
	options
}

func (c *client) gsmSecretName(clusterSecret *corev1.Secret) string {
	// Use cluster name, namespace and secret name is almost unique identifier.
	// However, if consider GCP allow creating clusters with the same name under
	// different zones, probably will need to add zones to this. Will address if
	// ever needed.
	return fmt.Sprintf("%s__%s__%s", c.clusterContext, clusterSecret.Namespace, clusterSecret.Name)
}

// gatherOptions parses the command-line flags.
func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.StringVar(&o.project, "project", "", "GCP project used for backing up secrets")
	fs.StringVar(&o.clusterContext, "cluster-context", "", "cluster context name used for backing up secrets, must be full form such as <PROVIDER>_<PROJECT>_<ZONE>_<CLUSTER>")
	fs.StringSliceVar(&o.namespaces, "namespace", []string{}, "namespace to backup, can be passed in repeatedly")
	fs.StringToStringVar(&o.secrets, "secret-name", nil, "namespace:name of secrets to be backed up, in the form of --secret-name=<namespace>=<name>. By default all secrets in the chosen namespace(s) are backed up.")
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
	if len(o.clusterContext) == 0 {
		return errors.New("--cluster-context must be provided")
	}
	return nil
}

func newClient(o options) (*client, error) {
	secretmanagerClient, err := secretmanager.NewClient(o.project, o.dryRun)
	if err != nil {
		return nil, fmt.Errorf("failed creating secret manager client: %w", err)
	}

	kubeClient, err := newKubeClients(o.clusterContext)
	if err != nil {
		return nil, fmt.Errorf("failed creating kube client: %w", err)
	}
	return &client{
		kubeClient:          kubeClient,
		secretmanagerClient: secretmanagerClient,
		options:             o,
	}, nil
}

func newKubeClients(clusterContext string) (ctrlruntimeclient.Client, error) {
	var loader clientcmd.ClientConfigLoader
	loader = clientcmd.NewDefaultClientConfigLoadingRules()

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loader, &clientcmd.ConfigOverrides{
			// Enforcing clusterContext in the full form of cluster context,
			// instead of short names for kubectl.
			Context:        api.Context{Cluster: clusterContext},
			CurrentContext: clusterContext,
		}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed create rest config: %v ------ Did you supply the full form of cluster context name instead of short hand?", err)
	}
	return ctrlruntimeclient.New(cfg, ctrlruntimeclient.Options{})
}

// process merges secret into a new secret for write.
func (c *client) updateSingleSecret(ctx context.Context, clusterSecret *corev1.Secret) error {
	secretID := c.gsmSecretName(clusterSecret)
	// Google secret manager expects pure string instead of map[string][]byte,
	// so translate *corev1.Secret.Data to map[string]string, this will also be
	// what kubernetes external secret expects.
	stringData := map[string]string{}
	for key, val := range clusterSecret.Data {
		stringData[key] = string(val)
	}
	payload, err := json.Marshal(stringData)
	if err != nil {
		return fmt.Errorf("failed marshal secret %s: %w", clusterSecret.Name, err)
	}
	log := logrus.WithFields(logrus.Fields{
		"project":     c.project,
		"cluster":     c.clusterContext,
		"namespace":   clusterSecret.Namespace,
		"secret-name": clusterSecret.Name,
		"gsm-secret":  secretID,
	})
	if sat := "kubernetes.io/service-account-token"; string(clusterSecret.Type) == sat {
		log.Infof("Skipping: the secret type is %s", sat)
		return nil
	}
	if c.dryRun {
		log.Info("[Dryrun]: backing up secret")
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
	if err := c.secretmanagerClient.AddSecretVersion(ctx, secretID, payload); err != nil {
		return err
	}
	return c.secretmanagerClient.AddSecretLabel(ctx, secretID, defaultSecretLabels)
}

func (c *client) updateAllSecrets(ctx context.Context, allowed map[string]string) error {
	for _, secret := range c.allSi.Items {
		if allowed != nil {
			if val, ok := allowed[secret.Namespace]; !ok || val != secret.Name {
				continue
			}
		}
		if err := c.updateSingleSecret(ctx, &secret); err != nil {
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
	defaultSecretLabels["source"] = o.clusterContext

	ctx := context.Background()

	c, err := newClient(o)
	if err != nil {
		logrus.WithError(err).Fatal("Failed creating client")
	}

	if err := c.loadClusterSecrets(ctx); err != nil {
		logrus.WithError(err).Fatal("Failed listing secrets")
	}

	if err := c.updateAllSecrets(ctx, o.secrets); err != nil {
		logrus.WithError(err).Fatal("Failed updating secrets")
	}
}
