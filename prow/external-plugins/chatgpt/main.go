/*
Copyright 2017 The Kubernetes Authors.

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
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/config/secret"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"
	"k8s.io/test-infra/prow/pluginhelp/externalplugins"
)

type options struct {
	port int

	openaiConfigFile           string
	openaiConfigFileLarge      string
	openaiTasksFile            string
	openaiConfigReloadInterval time.Duration
	openaiTasksReloadInterval  time.Duration

	largeDownThreshold  int
	maxAcceptDiffSize   int
	issueCommentCommand string

	dryRun                 bool
	github                 prowflagutil.GitHubOptions
	instrumentationOptions prowflagutil.InstrumentationOptions
	logLevel               string

	webhookSecretFile string
}

func (o *options) Validate() error {
	for idx, group := range []flagutil.OptionGroup{&o.github} {
		if err := group.Validate(o.dryRun); err != nil {
			return fmt.Errorf("%d: %w", idx, err)
		}
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.IntVar(&o.port, "port", 8888, "Port to listen on.")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	fs.StringVar(&o.webhookSecretFile, "hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	fs.StringVar(&o.openaiConfigFile, "openai-config-file", "/etc/openai/config.yaml", "Path to the file containing the access credential.")
	fs.StringVar(&o.openaiConfigFileLarge, "openai-config-file-large", "", "Path to the file containing the access credential route for large pull requests.")
	fs.DurationVar(&o.openaiConfigReloadInterval, "openai-config-reload-interval", time.Minute, "Interval to reload the openai access credential file.")
	fs.StringVar(&o.openaiTasksFile, "openai-tasks-file", "/etc/openai/tasks.yaml", "Path to the file containing the default openai tasks.")
	fs.DurationVar(&o.openaiTasksReloadInterval, "openai-tasks-reload-interval", time.Minute, "Interval to reload the openai tasks file.")
	fs.IntVar(&o.largeDownThreshold, "large-down-threshold", 3*4096, "down threshold bytes of message will route to client given by `openai-config-file-large` option.")
	fs.IntVar(&o.maxAcceptDiffSize, "max-accept-diff-size", 80000, "maximum bytes of PR diff")
	fs.StringVar(&o.issueCommentCommand, "issue-comment-command", "review", "comment command to match for, such as `command1` (you should send comment with `/command1 ...`)")
	fs.StringVar(&o.logLevel, "log-level", "debug", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))
	for _, group := range []flagutil.OptionGroup{&o.github, &o.instrumentationOptions} {
		group.AddFlags(fs)
	}
	fs.Parse(os.Args[1:])
	return o
}

func main() {
	logrusutil.ComponentInit()
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	logLevel, err := logrus.ParseLevel(o.logLevel)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to parse loglevel")
	}
	logrus.SetLevel(logLevel)
	log := logrus.StandardLogger().WithField("plugin", pluginName)

	if err := secret.Add(o.webhookSecretFile); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	githubClient, err := o.github.GitHubClient(o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}

	openaiAgent, err := NewWrapOpenaiAgent(o.openaiConfigFile, o.openaiConfigFileLarge, o.largeDownThreshold, o.openaiConfigReloadInterval)
	if err != nil {
		logrus.WithError(err).Fatal("Error load OpenAI config.")
	}

	taskAgent, err := NewTaskAgent(o.openaiTasksFile, o.openaiTasksReloadInterval)
	if err != nil {
		logrus.WithError(err).Fatal("Failed to start task agent")
	}

	issueCommentMatchRegex := regexp.MustCompile(fmt.Sprintf(`(?m)^/%s\s+(.+)$`, o.issueCommentCommand))
	server := &Server{
		ghc:                    githubClient,
		issueCommentMatchRegex: issueCommentMatchRegex,
		log:                    log,
		openaiClientAgent:      openaiAgent,
		openaiTaskAgent:        taskAgent,
		maxDiffSize:            o.maxAcceptDiffSize,
		tokenGenerator:         secret.GetTokenGenerator(o.webhookSecretFile),
	}

	health := pjutil.NewHealthOnPort(o.instrumentationOptions.HealthPort)
	health.ServeReady()

	mux := http.NewServeMux()
	mux.Handle("/", server)
	externalplugins.ServeExternalPluginHelp(mux, log, HelpProviderFactory(o.issueCommentCommand))
	httpServer := &http.Server{Addr: ":" + strconv.Itoa(o.port), Handler: mux}
	defer interrupts.WaitForGracefulShutdown()
	interrupts.ListenAndServe(httpServer, 5*time.Second)
}
