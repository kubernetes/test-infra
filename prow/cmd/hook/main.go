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
	"os/signal"
	"strconv"
	"syscall"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/hook"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/slack"

	_ "k8s.io/test-infra/prow/plugins/assign"
	_ "k8s.io/test-infra/prow/plugins/cla"
	_ "k8s.io/test-infra/prow/plugins/close"
	_ "k8s.io/test-infra/prow/plugins/heart"
	_ "k8s.io/test-infra/prow/plugins/label"
	_ "k8s.io/test-infra/prow/plugins/lgtm"
	_ "k8s.io/test-infra/prow/plugins/releasenote"
	_ "k8s.io/test-infra/prow/plugins/reopen"
	_ "k8s.io/test-infra/prow/plugins/trigger"
	_ "k8s.io/test-infra/prow/plugins/update_config"
	_ "k8s.io/test-infra/prow/plugins/yuks"
)

var (
	port = flag.Int("port", 8888, "Port to listen on.")

	configPath   = flag.String("config-path", "/etc/config/config", "Path to config.yaml.")
	pluginConfig = flag.String("plugin-config", "/etc/plugins/plugins", "Path to plugin config file.")

	local  = flag.Bool("local", false, "Run locally for testing purposes only. Does not require secret files.")
	dryRun = flag.Bool("dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")

	githubBotName     = flag.String("github-bot-name", "", "Name of the GitHub bot.")
	webhookSecretFile = flag.String("hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	githubTokenFile   = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth secret.")
	slackTokenFile    = flag.String("slack-token-file", "", "Path to the file containing the Slack Kubernetes Team Token.")
)

func main() {
	flag.Parse()

	var webhookSecret []byte
	var githubClient *github.Client
	var kubeClient *kube.Client
	var slackClient *slack.Client
	if *local {
		logrus.Warning("Running in local mode for dev only.")

		logrus.Print("HMAC Secret: abcde12345")
		webhookSecret = []byte("abcde12345")

		if *githubBotName == "" {
			*githubBotName = "fake-robot"
		}
		githubClient = github.NewFakeClient(*githubBotName)
		githubClient.Logger = logrus.StandardLogger()

		kubeClient = kube.NewFakeClient()
		kubeClient.Logger = logrus.StandardLogger()
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

		if *githubBotName == "" {
			logrus.Fatal("Must specify --github-bot-name.")
		}
		if *dryRun {
			githubClient = github.NewDryRunClient(*githubBotName, oauthSecret)
		} else {
			githubClient = github.NewClient(*githubBotName, oauthSecret)
		}

		kubeClient, err = kube.NewClientInCluster(kube.ProwNamespace)
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

	configAgent := &config.Agent{}
	if err := configAgent.Start(*configPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	pluginAgent := &plugins.PluginAgent{
		PluginClient: plugins.PluginClient{
			GitHubClient: githubClient,
			KubeClient:   kubeClient,
			SlackClient:  slackClient,
			Logger:       logrus.NewEntry(logrus.StandardLogger()),
		},
	}
	if err := pluginAgent.Start(*pluginConfig); err != nil {
		logrus.WithError(err).Fatal("Error starting plugins.")
	}

	server := &hook.Server{
		HMACSecret:  webhookSecret,
		ConfigAgent: configAgent,
		Plugins:     pluginAgent,
	}

	// Return 200 on / for health checks.
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})
	// For /hook, handle a webhook normally.
	http.Handle("/hook", server)
	logrus.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}
