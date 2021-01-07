/*
Copyright 2020 The Kubernetes Authors.

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
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	flag "github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth" // Enable all auth provider plugins
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/clientcmd/api/latest"
	"k8s.io/test-infra/gencred/pkg/util"
	"k8s.io/test-infra/mergecred/pkg/kubeconfig"
	"k8s.io/test-infra/mergecred/pkg/secretmanager"
	"sigs.k8s.io/yaml"
)

const (
	// defaultContextName is the default context name.
	defaultContextName = "build"
	// defaultNamespace is the default namespace for the secret.
	defaultNamespace = "default"
	// defaultConfigFileName is the default kubeconfig filename.
	defaultConfigFileName = "/dev/stdout"
	defaultSecretID       = "prow-kubeconfig-backup"
)

var reAutoKey = regexp.MustCompile(`^config\-[0-9]{8}$`)

// options are the available command-line flags.
type options struct {
	help              bool
	project           string
	context           string
	name              string
	namespace         string
	srcKey            string
	dstKey            string
	kubeconfigToMerge string
	prune             bool
	auto              bool
}

// parseFlags parses the command-line flags.
func (o *options) parseFlags() {
	flag.BoolVar(&o.help, "help", false, "Merges the provided kubeconfig file into a kubeconfig file living in a kubernetes secret in order to add new cluster contexts to the secret. Requires kubectl and base64.")
	flag.StringVar(&o.project, "project", "", "GCP project for backing up old secret.")
	flag.StringVar(&o.context, "context", "", "The name of the kubeconfig context to use.")
	flag.StringVar(&o.name, "name", "kubeconfig", "The name of the k8s secret containing the kubeconfig file to add to.")
	flag.StringVar(&o.namespace, "namespace", defaultNamespace, "Context name for the kubeconfig entry.")
	flag.StringVar(&o.srcKey, "src-key", "", "The key of the source kubeconfig file in the k8s secret.")
	flag.StringVar(&o.dstKey, "dst-key", "", "The destination key of the merged kubeconfig file in the k8s secret.")
	flag.StringVar(&o.kubeconfigToMerge, "kubeconfig-to-merge", "", "Filepath of the kubeconfig file to merge into the kubeconfig secret.")
	flag.BoolVar(&o.prune, "prune", true, "Remove all secret keys besides the source and dest. This should be used periodically to delete old kubeconfigs and keep the secret size under control.")
	flag.BoolVar(&o.auto, "auto", true, "Automatically determine --dest-key and optionally --src-key assuming keys are of the form 'config-20200730'. Pruning is enabled.")

	flag.Parse()
}

// validateFlags validates the command-line flags.
func (o *options) validateFlags() error {
	var err error

	if len(o.project) == 0 {
		return errors.New("--project option is required")
	}

	if len(o.context) == 0 {
		return errors.New("--context option is required")
	}

	if o.auto {
		if len(o.dstKey) > 0 {
			return errors.New("--dest-key must be omitted when --auto is used")
		}
	} else {
		if len(o.srcKey) == 0 || len(o.dstKey) == 0 {
			return errors.New("--src-key and --dest-key are required unless --auto is used")
		}
	}

	o.kubeconfigToMerge, err = filepath.Abs(o.kubeconfigToMerge)
	if err != nil {
		return fmt.Errorf("--kubeconfig-to-merge option invalid: %v", o.kubeconfigToMerge)
	}

	if !util.FileExists(o.kubeconfigToMerge) {
		return fmt.Errorf("--kubeconfig-to-merge not exists: %q", o.kubeconfigToMerge)
	}

	return nil
}

// mergeConfigs merges an existing kubeconfig file with a new entry with precedence given to the existing config.
func mergeConfigs(kubeconfig []byte, newFile string) ([]byte, error) {
	tmpFile, err := ioutil.TempFile("", "")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmpFile.Name())

	err = ioutil.WriteFile(tmpFile.Name(), kubeconfig, 0644)
	if err != nil {
		return nil, err
	}

	loadingRules := clientcmd.ClientConfigLoadingRules{
		Precedence: []string{tmpFile.Name(), newFile},
	}

	mergedConfig, err := loadingRules.Load()
	if err != nil {
		return nil, err
	}

	json, err := runtime.Encode(latest.Codec, mergedConfig)
	if err != nil {
		return nil, err
	}

	kubeconfig, err = yaml.JSONToYAML(json)
	if err != nil {
		return nil, err
	}

	return kubeconfig, nil
}

func getKeys(secretMap map[string][]byte, o options) (string, string, bool, error) {
	srcKey, dstKey, prune := o.srcKey, o.dstKey, o.prune
	if o.auto {
		dstKey = fmt.Sprintf("config-%s", time.Now().Format("20060102"))
		if len(o.srcKey) == 0 {
			var keys []string
			var validKeys []string
			for key := range secretMap {
				keys = append(keys, key)
				if matches := reAutoKey.FindStringSubmatch(key); len(matches) > 0 {
					validKeys = append(validKeys, matches[0])
				}
			}
			sort.Strings(validKeys)
			if len(validKeys) == 0 {
				return "", "", false,
					fmt.Errorf("The secret does not contain any keys matching the 'config-20200730' format: '%v'. Please try again with --src-key set to the most recent key", keys)
			}
			srcKey = validKeys[len(validKeys)-1]
			// Only enable pruning if we won't overwrite the source key.
			// This ensures that a second update on the same day will still have a
			// key to roll back to if needed.
			prune = (srcKey != dstKey)
			log.Printf("Automatic mode: --src-key=%s  --dest-key=%s", srcKey, dstKey)
		}
	}
	return srcKey, dstKey, prune, nil
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
func process(ctx context.Context, o options, clientset kubernetes.Interface, secretmanagerClient secretmanager.ClientInterface) (*corev1.Secret, error) {
	// kubeconfig is a kubernetes config.
	var srcKubeconfig []byte

	orig, err := clientset.CoreV1().Secrets(o.namespace).Get(context.Background(), o.name, v1.GetOptions{})
	if err != nil {
		return nil, err
	}

	srcKey, dstKey, prune, err := getKeys(orig.Data, o)
	if err != nil {
		return nil, err
	}
	srcKubeconfig = orig.Data[srcKey]

	// TODO: save secret to GCP secret manager.
	body, err := orig.Marshal()
	if err != nil {
		return nil, err
	}
	log.Printf("Secret backed up at %s of project %s", defaultSecretID, secretmanagerClient.Project())
	if err = backupSecret(ctx, secretmanagerClient, defaultSecretID, body); err != nil {
		return nil, err
	}

	dstKubeconfig, err := mergeConfigs(srcKubeconfig, o.kubeconfigToMerge)
	if err != nil {
		return nil, err
	}
	if prune {
		orig.Data = map[string][]byte{
			srcKey: srcKubeconfig,
			dstKey: dstKubeconfig,
		}
	} else {
		orig.Data[dstKey] = dstKubeconfig
	}

	return orig, nil
}

// Main entry point.
func Main() {
	var o options

	o.parseFlags()
	if err := o.validateFlags(); err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	clientset, err := kubeconfig.NewKubeClient(o.context)
	if err != nil {
		log.Fatal(err)
	}

	secretmanagerClient, err := secretmanager.NewClient(o.project)
	if err != nil {
		log.Fatal(err)
	}

	merged, err := process(ctx, o, clientset, secretmanagerClient)
	if err != nil {
		log.Fatal(err)
	}

	if _, err := clientset.CoreV1().Secrets(defaultNamespace).Update(ctx, merged, v1.UpdateOptions{}); err != nil {
		log.Fatal(err)
	}
}
