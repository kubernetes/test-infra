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
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp" // support gcp users in .kube/config
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
	kubernetes prowflagutil.LegacyKubernetesOptions
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

	_, defaultContext, kubernetesClients, err := o.kubernetes.Client(configAgent.Config().ProwJobNamespace, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Kubernetes client.")
	}

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
		if cm.Namespace == "" {
			cm.Namespace = configAgent.Config().ProwJobNamespace
		}
		logger := logrus.WithFields(logrus.Fields{"configmap": map[string]string{"name": cm.Name, "namespace": cm.Namespace}})
		if err := updateconfig.Update(&osFileGetter{root: o.sourcePath}, kubernetesClients[defaultContext].CoreV1().ConfigMaps(cm.Namespace), cm.Name, cm.Namespace, data, logger); err != nil {
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
