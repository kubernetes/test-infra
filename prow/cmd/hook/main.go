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
	"bytes"
	"context"
	"errors"
	"flag"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/hook"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	pluginhelp "k8s.io/test-infra/prow/pluginhelp/hook"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/repoowners"
	"k8s.io/test-infra/prow/slack"
)

type options struct {
	port int

	configPath   string
	cluster      string
	pluginConfig string

	dryRun      bool
	gracePeriod time.Duration
	deckURL     string

	githubEndpoint  flagutil.Strings
	githubTokenFile string

	webhookSecretFile string
	slackTokenFile    string
}

func (o *options) Validate() error {
	if o.dryRun && o.deckURL == "" {
		return errors.New("a dry-run was requested but required flag --deck-url was unset")
	}
	return nil
}

func gatherOptions() options {
	o := options{
		githubEndpoint: flagutil.NewStrings("https://api.github.com"),
	}
	flag.IntVar(&o.port, "port", 8888, "Port to listen on.")

	flag.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	flag.StringVar(&o.cluster, "cluster", "", "Path to kube.Cluster YAML file. If empty, uses the local cluster.")
	flag.StringVar(&o.pluginConfig, "plugin-config", "/etc/plugins/plugins.yaml", "Path to plugin config file.")

	flag.BoolVar(&o.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	flag.DurationVar(&o.gracePeriod, "grace-period", 180*time.Second, "On shutdown, try to handle remaining events for the specified duration. ")
	flag.StringVar(&o.deckURL, "deck-url", "", "Deck URL for read-only access to the cluster.")

	flag.Var(&o.githubEndpoint, "github-endpoint", "GitHub's API endpoint.")
	flag.StringVar(&o.githubTokenFile, "github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth secret.")

	flag.StringVar(&o.webhookSecretFile, "hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	flag.StringVar(&o.slackTokenFile, "slack-token-file", "", "Path to the file containing the Slack token to use.")
	flag.Parse()
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}
	logrus.SetFormatter(logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "hook"}))

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, ""); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	webhookSecretRaw, err := ioutil.ReadFile(o.webhookSecretFile)
	if err != nil {
		logrus.WithError(err).Fatal("Could not read webhook secret file.")
	}
	webhookSecret := bytes.TrimSpace(webhookSecretRaw)

	oauthSecretRaw, err := ioutil.ReadFile(o.githubTokenFile)
	if err != nil {
		logrus.WithError(err).Fatal("Could not read oauth secret file.")
	}
	oauthSecret := string(bytes.TrimSpace(oauthSecretRaw))

	var teamToken string
	if o.slackTokenFile != "" {
		teamTokenRaw, err := ioutil.ReadFile(o.slackTokenFile)
		if err != nil {
			logrus.WithError(err).Fatal("Could not read slack token file.")
		}
		teamToken = string(bytes.TrimSpace(teamTokenRaw))
	}

	for _, ep := range o.githubEndpoint.Strings() {
		_, err = url.Parse(ep)
		if err != nil {
			logrus.WithError(err).Fatalf("Invalid --endpoint URL %q.", ep)
		}
	}

	var githubClient *github.Client
	var kubeClient *kube.Client
	if o.dryRun {
		githubClient = github.NewDryRunClient(oauthSecret, o.githubEndpoint.Strings()...)
		kubeClient = kube.NewFakeClient(o.deckURL)
	} else {
		githubClient = github.NewClient(oauthSecret, o.githubEndpoint.Strings()...)
		if o.cluster == "" {
			kubeClient, err = kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
			if err != nil {
				logrus.WithError(err).Fatal("Error getting kube client.")
			}
		} else {
			kubeClient, err = kube.NewClientFromFile(o.cluster, configAgent.Config().ProwJobNamespace)
			if err != nil {
				logrus.WithError(err).Fatal("Error getting kube client.")
			}
		}
	}

	var slackClient *slack.Client
	if !o.dryRun && teamToken != "" {
		logrus.Info("Using real slack client.")
		slackClient = slack.NewClient(teamToken)
	}
	if slackClient == nil {
		logrus.Info("Using fake slack client.")
		slackClient = slack.NewFakeClient()
	}

	gitClient, err := git.NewClient()
	if err != nil {
		logrus.WithError(err).Fatal("Error getting git client.")
	}
	defer gitClient.Clean()
	// Get the bot's name in order to set credentials for the git client.
	botName, err := githubClient.BotName()
	if err != nil {
		logrus.WithError(err).Fatal("Error getting bot name.")
	}
	gitClient.SetCredentials(botName, oauthSecret)

	pluginAgent := &plugins.PluginAgent{}

	ownersClient := repoowners.NewClient(
		gitClient, githubClient,
		configAgent, pluginAgent.MDYAMLEnabled,
		pluginAgent.SkipCollaborators,
	)

	pluginAgent.PluginClient = plugins.PluginClient{
		GitHubClient: githubClient,
		KubeClient:   kubeClient,
		GitClient:    gitClient,
		SlackClient:  slackClient,
		OwnersClient: ownersClient,
		Logger:       logrus.WithField("agent", "plugin"),
	}
	if err := pluginAgent.Start(o.pluginConfig); err != nil {
		logrus.WithError(err).Fatal("Error starting plugins.")
	}

	promMetrics := hook.NewMetrics()

	// Push metrics to the configured prometheus pushgateway endpoint.
	pushGateway := configAgent.Config().PushGateway
	if pushGateway.Endpoint != "" {
		go metrics.PushMetrics("hook", pushGateway.Endpoint, pushGateway.Interval)
	}

	server := &hook.Server{
		HMACSecret:  webhookSecret,
		ConfigAgent: configAgent,
		Plugins:     pluginAgent,
		Metrics:     promMetrics,
	}
	defer server.GracefulShutdown()

	// Return 200 on / for health checks.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})
	http.Handle("/metrics", promhttp.Handler())
	// For /hook, handle a webhook normally.
	http.Handle("/hook", server)
	// Serve plugin help information from /plugin-help.
	http.Handle("/plugin-help", pluginhelp.NewHelpAgent(pluginAgent, githubClient))

	httpServer := &http.Server{Addr: ":" + strconv.Itoa(o.port)}

	// Shutdown gracefully on SIGTERM or SIGINT
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		logrus.Info("Hook is shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), o.gracePeriod)
		defer cancel()
		httpServer.Shutdown(ctx)
	}()

	logrus.WithError(httpServer.ListenAndServe()).Warn("Server exited.")
}
