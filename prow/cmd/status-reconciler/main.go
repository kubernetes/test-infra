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
	"context"
	"flag"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/pjutil/pprof"

	"k8s.io/test-infra/pkg/flagutil"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	configflagutil "k8s.io/test-infra/prow/flagutil/config"
	pluginsflagutil "k8s.io/test-infra/prow/flagutil/plugins"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/statusreconciler"
)

const (
	defaultTokens = 300
	defaultBurst  = 100
)

type options struct {
	config        configflagutil.ConfigOptions
	pluginsConfig pluginsflagutil.PluginOptions

	continueOnError           bool
	addedPresubmitDenylist    prowflagutil.Strings
	addedPresubmitDenylistAll prowflagutil.Strings
	dryRun                    bool
	kubernetes                prowflagutil.KubernetesOptions
	github                    prowflagutil.GitHubOptions
	storage                   prowflagutil.StorageClientOptions
	instrumentationOptions    prowflagutil.InstrumentationOptions

	// statusURI where Status-reconciler stores last known state, i.e. configuration.
	// Can be /local/path, gs://path/to/object or s3://path/to/object.
	// GCS writes will use the bucket's default acl for new objects. Ensure both that
	// a) the gcs credentials can write to this bucket
	// b) the default acls do not expose any private info
	statusURI string
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	o := options{config: configflagutil.ConfigOptions{ConfigPath: "/etc/config/config.yaml"}}

	fs.StringVar(&o.statusURI, "status-path", "", "The /local/path, gs://path/to/object or s3://path/to/object to store status controller state. GCS writes will use the default object ACL for the bucket.")

	fs.BoolVar(&o.continueOnError, "continue-on-error", false, "Indicates that the migration should continue if context migration fails for an individual PR.")
	fs.Var(&o.addedPresubmitDenylist, "denylist", "Org or org/repo to ignore new added presubmits for, set more than once to add more.")
	fs.Var(&o.addedPresubmitDenylistAll, "denylist-all", "Org or org/repo to ignore reconciling, set more than once to add more.")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to GitHub.")
	o.github.AddCustomizedFlags(fs, prowflagutil.ThrottlerDefaults(defaultTokens, defaultBurst))
	o.pluginsConfig.PluginConfigPathDefault = "/etc/plugins/plugins.yaml"
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.storage, &o.instrumentationOptions, &o.config, &o.pluginsConfig} {
		group.AddFlags(fs)
	}
	fs.Parse(args)
	return o
}

func (o *options) Validate() error {
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github, &o.storage, &o.config, &o.pluginsConfig} {
		if err := group.Validate(o.dryRun); err != nil {
			return err
		}
	}

	return nil
}

func (o *options) getDenyList() sets.Set[string] {
	denyList := o.addedPresubmitDenylist.Strings()

	return sets.New[string](denyList...)
}

func (o *options) getDenyListAll() sets.Set[string] {
	denyListAll := o.addedPresubmitDenylistAll.Strings()
	return sets.New[string](denyListAll...)
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	defer interrupts.WaitForGracefulShutdown()

	pprof.Instrument(o.instrumentationOptions)

	configAgent, err := o.config.ConfigAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	pluginAgent, err := o.pluginsConfig.PluginAgent()
	if err != nil {
		logrus.WithError(err).Fatal("Error starting plugin configuration agent.")
	}

	githubClient, err := o.github.GitHubClient(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}

	prowJobClient, err := o.kubernetes.ProwJobClient(configAgent.Config().ProwJobNamespace, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	opener, err := o.storage.StorageClient(ctx)
	if err != nil {
		logrus.WithError(err).Fatal("Cannot create opener")
	}

	c := statusreconciler.NewController(o.continueOnError, o.getDenyList(), o.getDenyListAll(), opener, o.config, o.statusURI, prowJobClient, githubClient, pluginAgent)
	interrupts.Run(func(ctx context.Context) {
		c.Run(ctx)
	})
}
