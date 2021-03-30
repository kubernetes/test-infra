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

	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/secret"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/statusreconciler"
)

const (
	defaultTokens = 300
	defaultBurst  = 100
)

type options struct {
	configPath                 string
	jobConfigPath              string
	supplementalProwConfigDirs prowflagutil.Strings
	pluginConfig               string

	continueOnError           bool
	addedPresubmitDenylist    prowflagutil.Strings
	addedPresubmitDenylistAll prowflagutil.Strings
	dryRun                    bool
	kubernetes                prowflagutil.KubernetesOptions
	github                    prowflagutil.GitHubOptions
	storage                   prowflagutil.StorageClientOptions
	instrumentationOptions    prowflagutil.InstrumentationOptions

	tokenBurst    int
	tokensPerHour int
	// statusURI where Status-reconciler stores last known state, i.e. configuration.
	// Can be /local/path, gs://path/to/object or s3://path/to/object.
	// GCS writes will use the bucket's default acl for new objects. Ensure both that
	// a) the gcs credentials can write to this bucket
	// b) the default acls do not expose any private info
	statusURI string
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	o := options{}

	fs.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.Var(&o.supplementalProwConfigDirs, "supplemental-prow-config-dir", "An additional directory from which to load prow configs. Can be used for config sharding but only supports a subset of the config. The flag can be passed multiple times.")
	fs.StringVar(&o.pluginConfig, "plugin-config", "/etc/plugins/plugins.yaml", "Path to plugin config file.")
	fs.StringVar(&o.statusURI, "status-path", "", "The /local/path, gs://path/to/object or s3://path/to/object to store status controller state. GCS writes will use the default object ACL for the bucket.")

	fs.BoolVar(&o.continueOnError, "continue-on-error", false, "Indicates that the migration should continue if context migration fails for an individual PR.")
	fs.Var(&o.addedPresubmitDenylist, "denylist", "Org or org/repo to ignore new added presubmits for, set more than once to add more.")
	fs.Var(&o.addedPresubmitDenylistAll, "denylist-all", "Org or org/repo to ignore reconciling, set more than once to add more.")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to GitHub.")
	fs.IntVar(&o.tokensPerHour, "tokens", defaultTokens, "Throttle hourly token consumption (0 to disable)")
	fs.IntVar(&o.tokenBurst, "token-burst", defaultBurst, "Allow consuming a subset of hourly tokens in a short burst")
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github, &o.storage, &o.instrumentationOptions} {
		group.AddFlags(fs)
	}
	fs.Parse(args)
	return o
}

func (o *options) Validate() error {
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github, &o.storage} {
		if err := group.Validate(o.dryRun); err != nil {
			return err
		}
	}

	return nil
}

func (o *options) getDenyList() sets.String {
	denyList := o.addedPresubmitDenylist.Strings()
	return sets.NewString(denyList...)
}

func (o *options) getDenyListAll() sets.String {
	denyListAll := o.addedPresubmitDenylistAll.Strings()
	return sets.NewString(denyListAll...)
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	defer interrupts.WaitForGracefulShutdown()

	pjutil.ServePProf(o.instrumentationOptions.PProfPort)

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath, o.supplementalProwConfigDirs.Strings()); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	secretAgent := &secret.Agent{}
	if o.github.TokenPath != "" {
		if err := secretAgent.Start([]string{o.github.TokenPath}); err != nil {
			logrus.WithError(err).Fatal("Error starting secrets agent.")
		}
	}

	pluginAgent := &plugins.ConfigAgent{}
	if err := pluginAgent.Start(o.pluginConfig, false); err != nil {
		logrus.WithError(err).Fatal("Error starting plugin configuration agent.")
	}

	githubClient, err := o.github.GitHubClient(secretAgent, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}
	if o.tokensPerHour > 0 {
		githubClient.Throttle(o.tokensPerHour, o.tokenBurst)
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

	c := statusreconciler.NewController(o.continueOnError, o.getDenyList(), o.getDenyListAll(), opener, o.configPath, o.jobConfigPath, o.supplementalProwConfigDirs.Strings(), o.statusURI, prowJobClient, githubClient, pluginAgent)
	interrupts.Run(func(ctx context.Context) {
		c.Run(ctx)
	})
}
