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
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/secret"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/hook"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	pluginhelp "k8s.io/test-infra/prow/pluginhelp/hook"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/slack"
)

type options struct {
	port int

	configPath    string
	jobConfigPath string
	pluginConfig  string

	dryRun      bool
	gracePeriod time.Duration
	kubernetes  prowflagutil.LegacyKubernetesOptions
	github      prowflagutil.GitHubOptions

	webhookSecretFile string
	slackTokenFile    string
}

func (o *options) Validate() error {
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github} {
		if err := group.Validate(o.dryRun); err != nil {
			return err
		}
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.IntVar(&o.port, "port", 8888, "Port to listen on.")

	fs.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	fs.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	fs.StringVar(&o.pluginConfig, "plugin-config", "/etc/plugins/plugins.yaml", "Path to plugin config file.")

	fs.BoolVar(&o.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	fs.DurationVar(&o.gracePeriod, "grace-period", 180*time.Second, "On shutdown, try to handle remaining events for the specified duration. ")
	for _, group := range []flagutil.OptionGroup{&o.kubernetes, &o.github} {
		group.AddFlags(fs)
	}

	fs.StringVar(&o.webhookSecretFile, "hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	fs.StringVar(&o.slackTokenFile, "slack-token-file", "", "Path to the file containing the Slack token to use.")
	fs.Parse(os.Args[1:])
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}
	logrus.SetFormatter(logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "hook"}))

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

	secretAgent := &secret.Agent{}
	if err := secretAgent.Start(tokens); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	githubClient, err := o.github.GitHubClient(secretAgent, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}
	gitClient, err := o.github.GitClient(secretAgent, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Git client.")
	}
	defer gitClient.Clean()

	kubeClient, defaultContext, kubernetesClients, err := o.kubernetes.Client(configAgent.Config().ProwJobNamespace, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting Kubernetes client.")
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

	clientAgent := &plugins.ClientAgent{
		GitHubClient:     githubClient,
		KubeClient:       kubeClient,
		KubernetesClient: kubernetesClients[defaultContext],
		GitClient:        gitClient,
		SlackClient:      slackClient,
	}

	pluginAgent := &plugins.ConfigAgent{}
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
		ClientAgent:    clientAgent,
		ConfigAgent:    configAgent,
		Plugins:        pluginAgent,
		Metrics:        promMetrics,
		TokenGenerator: secretAgent.GetTokenGenerator(o.webhookSecretFile),
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
