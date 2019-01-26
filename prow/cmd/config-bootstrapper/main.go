/*
Copyright 2018 The Kubernetes Authors.

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
	"errors"
	"flag"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/sirupsen/logrus"
	v1api "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // support gcp users in .kube/config
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/test-infra/prow/kube"

	"k8s.io/test-infra/prow/config"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	_ "k8s.io/test-infra/prow/hook"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/updateconfig"
)

type options struct {
	sourcePath string

	configPath    string
	jobConfigPath string
	pluginConfig  string

	dryRun     bool
	kubernetes prowflagutil.KubernetesOptions
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	fs.StringVar(&o.sourcePath, "source-path", "", "Path to root of source directory to use for config updates.")

	fs.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.pluginConfig, "plugin-config", "/etc/plugins/plugins.yaml", "Path to plugin config file.")

	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to GitHub.")
	o.kubernetes.AddFlags(fs)

	fs.Parse(os.Args[1:])
	return o
}

func (o *options) Validate() error {
	if o.sourcePath == "" {
		return errors.New("--source-path must be provided")
	}

	if err := o.kubernetes.Validate(o.dryRun); err != nil {
		return err
	}

	return nil
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "config-bootstrapper"}),
	)

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	pluginAgent := &plugins.ConfigAgent{}
	if err := pluginAgent.Start(o.pluginConfig); err != nil {
		logrus.WithError(err).Fatal("Error starting plugin configuration agent.")
	}

	credentials, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		logrus.WithError(err).Fatal("Could not load credentials from config.")
	}

	clusterConfig, err := clientcmd.NewDefaultClientConfig(*credentials, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		logrus.WithError(err).Fatal("Could not load client configuration.")
	}

	client, err := kubernetes.NewForConfig(clusterConfig)
	if err != nil {
		logrus.WithError(err).Fatal("Could not create Kubernetes client.")
	}

	kubeClient := adapter{client: client.CoreV1(), defaultNamespace: configAgent.Config().ProwJobNamespace}

	// act like the whole repo just got committed
	var changes []github.PullRequestChange
	filepath.Walk(o.sourcePath, func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		// we know path will be below sourcePath, but we can't
		// communicate that to the filepath module. We can ignore
		// this error as we can be certain it won't occur
		if relPath, err := filepath.Rel(o.sourcePath, path); err == nil {
			changes = append(changes, github.PullRequestChange{
				Filename: relPath,
				Status:   github.PullRequestFileAdded,
			})
		} else {
			logrus.WithError(err).Warn("unexpected error determining relative path to file")
		}
		return nil
	})

	for cm, data := range updateconfig.FilterChanges(pluginAgent.Config().ConfigUpdater.Maps, changes, logrus.NewEntry(logrus.StandardLogger())) {
		logger := logrus.WithFields(logrus.Fields{"configmap": map[string]string{"name": cm.Name, "namespace": cm.Namespace}})
		if err := updateconfig.Update(&osFileGetter{root: o.sourcePath}, &kubeClient, cm.Name, cm.Namespace, data, logger); err != nil {
			logger.WithError(err).Error("failed to update config on cluster")
		}
	}
}

type osFileGetter struct {
	root string
}

func (g *osFileGetter) GetFile(filename string) ([]byte, error) {
	return ioutil.ReadFile(filepath.Join(g.root, filename))
}

type adapter struct {
	defaultNamespace string
	client           corev1.CoreV1Interface
}

// GetConfigMap adapts the kube.Client GET method to the client-go equivalent
func (a *adapter) GetConfigMap(name, namespace string) (kube.ConfigMap, error) {
	if namespace == "" {
		namespace = a.defaultNamespace
	}
	output, err := a.client.ConfigMaps(namespace).Get(name, metav1.GetOptions{})
	return kube.ConfigMap(*output), err
}

// ReplaceConfigMap adapts the kube.Client PUT method to the client-go equivalent
func (a *adapter) ReplaceConfigMap(name string, config kube.ConfigMap) (kube.ConfigMap, error) {
	if config.Namespace == "" {
		config.Namespace = a.defaultNamespace
	}
	input := v1api.ConfigMap(config)
	output, err := a.client.ConfigMaps(input.Namespace).Update(&input)
	return kube.ConfigMap(*output), err
}

// CreateConfigMap adapts the kube.Client POST method to the client-go equivalent
func (a *adapter) CreateConfigMap(content kube.ConfigMap) (kube.ConfigMap, error) {
	if content.Namespace == "" {
		content.Namespace = a.defaultNamespace
	}
	input := v1api.ConfigMap(content)
	output, err := a.client.ConfigMaps(input.Namespace).Create(&input)
	return kube.ConfigMap(*output), err
}
