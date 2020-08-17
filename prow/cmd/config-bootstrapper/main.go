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
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // support gcp users in .kube/config

	"k8s.io/test-infra/prow/config"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	_ "k8s.io/test-infra/prow/hook/plugin-imports"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/updateconfig"
)

const bootstrapMode = true

type options struct {
	sourcePaths prowflagutil.Strings

	configPath    string
	jobConfigPath string
	pluginConfig  string

	dryRun     bool
	kubernetes prowflagutil.KubernetesOptions
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	fs.Var(&o.sourcePaths, "source-path", "Path to root of source directory to use for config updates. Can be set multiple times.")

	fs.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.pluginConfig, "plugin-config", "/etc/plugins/plugins.yaml", "Path to plugin config file.")

	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to GitHub.")
	o.kubernetes.AddFlags(fs)

	fs.Parse(os.Args[1:])
	return o
}

func (o *options) Validate() error {
	if len(o.sourcePaths.Strings()) == 0 {
		return errors.New("--source-path must be provided at least once")
	}

	if err := o.kubernetes.Validate(o.dryRun); err != nil {
		return err
	}

	return nil
}

type osFileGetter struct {
	roots []string
}

// GetFile returns the content of a file from disk, searching through all known roots.
// We assume that no two roots will contain the same relative path inside of them, as such
// a configuration would be racy and unsupported in the updateconfig plugin anyway.
func (g *osFileGetter) GetFile(filename string) ([]byte, error) {
	var loadErr error
	for _, root := range g.roots {
		candidatePath := filepath.Join(root, filename)
		if _, err := os.Stat(candidatePath); err == nil {
			// we found the file under this root
			return ioutil.ReadFile(candidatePath)
		} else if !os.IsNotExist(err) {
			// record this for later in case we can't find the file
			loadErr = err
		}
	}
	// file was found under no root
	return nil, loadErr
}

func run(sourcePaths []string, defaultNamespace string, configUpdater plugins.ConfigUpdater, client kubernetes.Interface, buildClusterCoreV1Clients map[string]corev1.CoreV1Interface) int {
	var errors int
	// act like the whole repo just got committed
	var changes []github.PullRequestChange
	for _, sourcePath := range sourcePaths {
		filepath.Walk(sourcePath, func(path string, info os.FileInfo, err error) error {
			if info.IsDir() {
				return nil
			}
			// we know path will be below sourcePaths, but we can't
			// communicate that to the filepath module. We can ignore
			// this error as we can be certain it won't occur
			if relPath, err := filepath.Rel(sourcePath, path); err == nil {
				changes = append(changes, github.PullRequestChange{
					Filename: relPath,
					Status:   github.PullRequestFileAdded,
				})
				logrus.Infof("added to mock change: %s", relPath)
			} else {
				logrus.WithError(err).Warn("unexpected error determining relative path to file")
				errors++
			}
			return nil
		})
	}

	for cm, data := range updateconfig.FilterChanges(configUpdater, changes, defaultNamespace, logrus.NewEntry(logrus.StandardLogger())) {
		logger := logrus.WithFields(logrus.Fields{"configmap": map[string]string{"name": cm.Name, "namespace": cm.Namespace, "cluster": cm.Cluster}})
		configMapClient, err := updateconfig.GetConfigMapClient(client.CoreV1(), cm.Namespace, buildClusterCoreV1Clients, cm.Cluster)
		if err != nil {
			errors++
			logrus.WithError(err).Errorf("Failed to find configMap client")
			continue
		}
		if err := updateconfig.Update(&osFileGetter{roots: sourcePaths}, configMapClient, cm.Name, cm.Namespace, data, bootstrapMode, nil, logger); err != nil {
			logger.WithError(err).Error("failed to update config on cluster")
			errors++
		} else {
			logger.Info("Successfully processed configmap")
		}
	}
	return errors
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	pluginAgent := &plugins.ConfigAgent{}
	if err := pluginAgent.Start(o.pluginConfig, true); err != nil {
		logrus.WithError(err).Fatal("Error starting plugin configuration agent.")
	}

	client, err := o.kubernetes.InfrastructureClusterClient(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Kubernetes client.")
	}

	buildClusterCoreV1Clients, err := o.kubernetes.BuildClusterCoreV1Clients(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Kubernetes clients for build cluster.")
	}

	if errors := run(o.sourcePaths.Strings(), configAgent.Config().ProwJobNamespace, pluginAgent.Config().ConfigUpdater, client, buildClusterCoreV1Clients); errors > 0 {
		logrus.WithField("fail-count", errors).Fatalf("errors occurred during update")
	}
}
