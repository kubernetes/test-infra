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
	"flag"
	"io/ioutil"
	"net/http"
	"net/url"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/hook"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/slack"
)

var (
	port = flag.Int("port", 8888, "Port to listen on.")

	configPath   = flag.String("config-path", "/etc/config/config", "Path to config.yaml.")
	pluginConfig = flag.String("plugin-config", "/etc/plugins/plugins", "Path to plugin config file.")

	local  = flag.Bool("local", false, "Run locally for testing purposes only. Does not require secret files.")
	dryRun = flag.Bool("dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")

	_               = flag.String("github-bot-name", "", "Deprecated.")
	githubEndpoint  = flag.String("github-endpoint", "https://api.github.com", "GitHub's API endpoint.")
	githubTokenFile = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth secret.")

	webhookSecretFile = flag.String("hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	slackTokenFile    = flag.String("slack-token-file", "", "Path to the file containing the Slack token to use.")
)

func main() {
	flag.Parse()

	configAgent := &config.Agent{}
	if err := configAgent.Start(*configPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}
	logger := logrus.StandardLogger()

	var webhookSecret []byte
	var githubClient *github.Client
	var kubeClient *kube.Client
	var slackClient *slack.Client
	if *local {
		logrus.Warning("Running in local mode for dev only.")

		logrus.Print("HMAC Secret: abcde12345")
		webhookSecret = []byte("abcde12345")

		githubClient = github.NewFakeClient()
		kubeClient = kube.NewFakeClient()
	} else {
		logrus.SetFormatter(&logrus.JSONFormatter{})

		// Ignore SIGTERM so that we don't drop hooks when the pod is removed.
		// We'll get SIGTERM first and then SIGKILL after our graceful termination
		// deadline.
		signal.Ignore(syscall.SIGTERM)

		webhookSecretRaw, err := ioutil.ReadFile(*webhookSecretFile)
		if err != nil {
			logrus.WithError(err).Fatal("Could not read webhook secret file.")
		}
		webhookSecret = bytes.TrimSpace(webhookSecretRaw)

		oauthSecretRaw, err := ioutil.ReadFile(*githubTokenFile)
		if err != nil {
			logrus.WithError(err).Fatal("Could not read oauth secret file.")
		}
		oauthSecret := string(bytes.TrimSpace(oauthSecretRaw))

		var teamToken string
		if *slackTokenFile != "" {
			teamTokenRaw, err := ioutil.ReadFile(*slackTokenFile)
			if err != nil {
				logrus.WithError(err).Fatal("Could not read slack token file.")
			}
			teamToken = string(bytes.TrimSpace(teamTokenRaw))
		}

		_, err = url.Parse(*githubEndpoint)
		if err != nil {
			logrus.WithError(err).Fatal("Must specify a valid --github-endpoint URL.")
		}

		if *dryRun {
			githubClient = github.NewDryRunClient(oauthSecret, *githubEndpoint)
		} else {
			githubClient = github.NewClient(oauthSecret, *githubEndpoint)
		}

		kubeClient, err = kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
		if err != nil {
			logrus.WithError(err).Fatal("Error getting kube client.")
		}

		if !*dryRun && teamToken != "" {
			logrus.Info("Using real slack client.")
			slackClient = slack.NewClient(teamToken)
		}
	}
	if slackClient == nil {
		logrus.Info("Using fake slack client.")
		slackClient = slack.NewFakeClient()
	}

	gitClient, err := git.NewClient()
	if err != nil {
		logrus.WithError(err).Fatal("Error getting git client.")
	}

	githubClient.Logger = logger.WithField("client", "github")
	kubeClient.Logger = logger.WithField("client", "kube")
	gitClient.Logger = logger.WithField("client", "git")

	pluginAgent := &plugins.PluginAgent{
		PluginClient: plugins.PluginClient{
			GitHubClient: githubClient,
			KubeClient:   kubeClient,
			GitClient:    gitClient,
			SlackClient:  slackClient,
			Logger:       logrus.NewEntry(logrus.StandardLogger()),
		},
	}
	if err := pluginAgent.Start(*pluginConfig); err != nil {
		logrus.WithError(err).Fatal("Error starting plugins.")
	}

	metrics, err := hook.NewMetrics()
	if err != nil {
		logrus.WithError(err).Fatal("Failed to initialize metrics.")
	}

	if configAgent.Config().PushGateway.Endpoint != "" {
		go func() {
			for {
				time.Sleep(time.Minute)
				if err := push.FromGatherer("hook", push.HostnameGroupingKey(), configAgent.Config().PushGateway.Endpoint, prometheus.DefaultGatherer); err != nil {
					logrus.WithError(err).Error("Failed to push metrics.")
				}
			}
		}()
	}

	server := &hook.Server{
		HMACSecret:  webhookSecret,
		ConfigAgent: configAgent,
		Plugins:     pluginAgent,
		Metrics:     metrics,
	}

	// Return 200 on / for health checks.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})
	http.Handle("/metrics", promhttp.Handler())
	// For /hook, handle a webhook normally.
	http.Handle("/hook", server)
	logrus.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}
