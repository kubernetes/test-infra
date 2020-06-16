/*
Copyright 2016 The Kubernetes Authors.

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
	"flag"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/bugzilla"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/secret"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/git/v2"
	"k8s.io/test-infra/prow/hook"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/pjutil"
	pluginhelp "k8s.io/test-infra/prow/pluginhelp/hook"
	"k8s.io/test-infra/prow/plugins"
	bzplugin "k8s.io/test-infra/prow/plugins/bugzilla"
	"k8s.io/test-infra/prow/repoowners"
	"k8s.io/test-infra/prow/slack"
)

type options struct {
	port int

	configPath    string
	jobConfigPath string
	pluginConfig  string

	dryRun                 bool
	gracePeriod            time.Duration
	kubernetes             prowflagutil.KubernetesOptions
	github                 prowflagutil.GitHubOptions
	bugzilla               prowflagutil.BugzillaOptions
	instrumentationOptions prowflagutil.InstrumentationOptions

	webhookSecretFile string
	slackTokenFile    string
}

func (o *options) Validate() error {
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github, &o.bugzilla} {
		if err := group.Validate(o.dryRun); err != nil {
			return err
		}
	}

	return nil
}

func gatherOptions(fs *flag.FlagSet, args ...string) options {
	var o options
	fs.IntVar(&o.port, "port", 8888, "Port to listen on.")

	fs.StringVar(&o.configPath, "config-path", "", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.pluginConfig, "plugin-config", "/etc/plugins/plugins.yaml", "Path to plugin config file.")

	fs.BoolVar(&o.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	fs.DurationVar(&o.gracePeriod, "grace-period", 180*time.Second, "On shutdown, try to handle remaining events for the specified duration. ")
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github, &o.bugzilla, &o.instrumentationOptions} {
		group.AddFlags(fs)
	}

	fs.StringVar(&o.webhookSecretFile, "hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	fs.StringVar(&o.slackTokenFile, "slack-token-file", "", "Path to the file containing the Slack token to use.")
	fs.Parse(args)
	return o
}

func main() {
	logrusutil.ComponentInit()

	o := gatherOptions(flag.NewFlagSet(os.Args[0], flag.ExitOnError), os.Args[1:]...)
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	var tokens []string

	// Append the path of hmac and github secrets.
	tokens = append(tokens, o.github.TokenPath)
	tokens = append(tokens, o.webhookSecretFile)

	// This is necessary since slack token is optional.
	if o.slackTokenFile != "" {
		tokens = append(tokens, o.slackTokenFile)
	}

	if o.bugzilla.ApiKeyPath != "" {
		tokens = append(tokens, o.bugzilla.ApiKeyPath)
	}

	secretAgent := &secret.Agent{}
	if err := secretAgent.Start(tokens); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	pluginAgent := &plugins.ConfigAgent{}
	if err := pluginAgent.Start(o.pluginConfig, true); err != nil {
		logrus.WithError(err).Fatal("Error starting plugins.")
	}

	githubClient, err := o.github.GitHubClient(secretAgent, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}
	gitClient, err := o.github.GitClient(secretAgent, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Git client.")
	}

	var bugzillaClient bugzilla.Client
	if orgs, repos := pluginAgent.Config().EnabledReposForPlugin(bzplugin.PluginName); orgs != nil || repos != nil {
		client, err := o.bugzilla.BugzillaClient(secretAgent)
		if err != nil {
			logrus.WithError(err).Fatal("Error getting Bugzilla client.")
		}
		bugzillaClient = client
	}

	infrastructureClient, err := o.kubernetes.InfrastructureClusterClient(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Kubernetes client for infrastructure cluster.")
	}

	buildClusterCoreV1Clients, err := o.kubernetes.BuildClusterCoreV1Clients(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Kubernetes clients for build cluster.")
	}

	prowJobClient, err := o.kubernetes.ProwJobClient(configAgent.Config().ProwJobNamespace, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting ProwJob client for infrastructure cluster.")
	}

	var slackClient *slack.Client
	if !o.dryRun && string(secretAgent.GetSecret(o.slackTokenFile)) != "" {
		logrus.Info("Using real slack client.")
		slackClient = slack.NewClient(secretAgent.GetTokenGenerator(o.slackTokenFile))
	}
	if slackClient == nil {
		logrus.Info("Using fake slack client.")
		slackClient = slack.NewFakeClient()
	}

	mdYAMLEnabled := func(org, repo string) bool {
		return pluginAgent.Config().MDYAMLEnabled(org, repo)
	}
	skipCollaborators := func(org, repo string) bool {
		return pluginAgent.Config().SkipCollaborators(org, repo)
	}
	ownersDirBlacklist := func() config.OwnersDirBlacklist {
		return configAgent.Config().OwnersDirBlacklist
	}
	ownersClient := repoowners.NewClient(git.ClientFactoryFrom(gitClient), githubClient, mdYAMLEnabled, skipCollaborators, ownersDirBlacklist)

	clientAgent := &plugins.ClientAgent{
		GitHubClient:              githubClient,
		ProwJobClient:             prowJobClient,
		KubernetesClient:          infrastructureClient,
		BuildClusterCoreV1Clients: buildClusterCoreV1Clients,
		GitClient:                 git.ClientFactoryFrom(gitClient),
		SlackClient:               slackClient,
		OwnersClient:              ownersClient,
		BugzillaClient:            bugzillaClient,
	}

	promMetrics := hook.NewMetrics()

	defer interrupts.WaitForGracefulShutdown()

	// Expose prometheus metrics
	metrics.ExposeMetrics("hook", configAgent.Config().PushGateway, o.instrumentationOptions.MetricsPort)
	pjutil.ServePProf(o.instrumentationOptions.PProfPort)

	server := &hook.Server{
		ClientAgent:    clientAgent,
		ConfigAgent:    configAgent,
		Plugins:        pluginAgent,
		Metrics:        promMetrics,
		TokenGenerator: secretAgent.GetTokenGenerator(o.webhookSecretFile),
	}
	interrupts.OnInterrupt(func() {
		server.GracefulShutdown()
		if err := gitClient.Clean(); err != nil {
			logrus.WithError(err).Error("Could not clean up git client cache.")
		}
	})

	health := pjutil.NewHealth()

	// TODO remove this health endpoint when the migration to health endpoint is done
	// Return 200 on / for health checks.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})

	// For /hook, handle a webhook normally.
	http.Handle("/hook", server)
	// Serve plugin help information from /plugin-help.
	http.Handle("/plugin-help", pluginhelp.NewHelpAgent(pluginAgent, githubClient))

	httpServer := &http.Server{Addr: ":" + strconv.Itoa(o.port)}

	health.ServeReady()

	interrupts.ListenAndServe(httpServer, o.gracePeriod)
}
