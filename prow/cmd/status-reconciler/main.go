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
	"k8s.io/test-infra/pkg/io"
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
	configPath    string
	jobConfigPath string
	pluginConfig  string

	continueOnError         bool
	addedPresubmitBlacklist prowflagutil.Strings
	dryRun                  bool
	kubernetes              prowflagutil.KubernetesOptions
	github                  prowflagutil.GitHubOptions

	tokenBurst    int
	tokensPerHour int

	// gcsCredentialsFile string is used for reading/writing to GCS block storage.
	// If you want to write to local paths, this parameter is optional.
	// If set, this file is used to read/write to gs:// paths
	// If not, credential auto-discovery is used
	gcsCredentialsFile string
	// 	s3CredentialsFile string is used for reading/writing to s3 block storage.
	// If you want to write to local paths, this parameter is optional.
	// If set, this file is used to read/write to s3:// paths
	// If not, go cloud credential auto-discovery is used
	// For more details see the pkg/io/providers pkg.
	s3CredentialsFile string
	// statusURI where Status-reconciler stores last known state, i.e. configuration.
	// Can be /local/path, gs://path/to/object or s3://path/to/object.
	// GCS writes will use the bucket's default acl for new objects. Ensure both that
	// a) the gcs credentials can write to this bucket
	// b) the default acls do not expose any private info
	statusURI string
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	fs.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.pluginConfig, "plugin-config", "/etc/plugins/plugins.yaml", "Path to plugin config file.")
	fs.StringVar(&o.gcsCredentialsFile, "gcs-credentials-file", "", "File where GCS credentials are stored")
	fs.StringVar(&o.s3CredentialsFile, "s3-credentials-file", "", "File where s3 credentials are stored. For the exact format see https://github.com/kubernetes/test-infra/blob/master/pkg/io/providers/providers.go")
	fs.StringVar(&o.statusURI, "status-path", "", "The /local/path, gs://path/to/object or s3://path/to/object to store status controller state. GCS writes will use the default object ACL for the bucket.")

	fs.BoolVar(&o.continueOnError, "continue-on-error", false, "Indicates that the migration should continue if context migration fails for an individual PR.")
	fs.Var(&o.addedPresubmitBlacklist, "blacklist", "Org or org/repo to ignore new added presubmits for, set more than once to add more.")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to GitHub.")
	fs.IntVar(&o.tokensPerHour, "tokens", defaultTokens, "Throttle hourly token consumption (0 to disable)")
	fs.IntVar(&o.tokenBurst, "token-burst", defaultBurst, "Allow consuming a subset of hourly tokens in a short burst")
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github} {
		group.AddFlags(fs)
	}

	fs.Parse(os.Args[1:])
	return o
}

func (o *options) Validate() error {
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github} {
		if err := group.Validate(o.dryRun); err != nil {
			return err
		}
	}

	return nil
}

func main() {
	logrusutil.ComponentInit("status-reconciler")

	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	defer interrupts.WaitForGracefulShutdown()

	pjutil.ServePProf()

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
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
	opener, err := io.NewOpener(ctx, o.gcsCredentialsFile, o.s3CredentialsFile)
	if err != nil {
		entry := logrus.WithError(err)
		if p := o.gcsCredentialsFile; p != "" {
			entry = entry.WithField("gcs-credentials-file", p)
		}
		if p := o.s3CredentialsFile; p != "" {
			entry = entry.WithField("s3-credentials-file", p)
		}
		entry.Fatal("Cannot create opener")
	}

	c := statusreconciler.NewController(o.continueOnError, sets.NewString(o.addedPresubmitBlacklist.Strings()...), opener, o.configPath, o.jobConfigPath, o.statusURI, prowJobClient, githubClient, pluginAgent)
	interrupts.Run(func(ctx context.Context) {
		c.Run(ctx)
	})
}
