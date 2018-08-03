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
	"net/http"
	"net/url"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp/externalplugins"
)

var (
	port              = flag.Int("port", 8888, "Port to listen on.")
	dryRun            = flag.Bool("dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	githubEndpoint    = flagutil.NewStrings("https://api.github.com")
	githubTokenFile   = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth secret.")
	webhookSecretFile = flag.String("hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	prowAssignments   = flag.Bool("use-prow-assignments", true, "Use prow commands to assign cherrypicked PRs.")
	allowAll          = flag.Bool("allow-all", false, "Allow anybody to use automated cherrypicks by skipping Github organization membership checks.")
)

func init() {
	flag.Var(&githubEndpoint, "github-endpoint", "GitHub's API endpoint.")
}

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})
	// TODO: Use global option from the prow config.
	logrus.SetLevel(logrus.DebugLevel)
	log := logrus.StandardLogger().WithField("plugin", "cherrypick")

	// Ignore SIGTERM so that we don't drop hooks when the pod is removed.
	// We'll get SIGTERM first and then SIGKILL after our graceful termination
	// deadline.
	signal.Ignore(syscall.SIGTERM)

	secretAgent := &config.SecretAgent{}
	if err := secretAgent.Start([]string{*githubTokenFile, *webhookSecretFile}); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	var err error
	for _, ep := range githubEndpoint.Strings() {
		_, err = url.ParseRequestURI(ep)
		if err != nil {
			logrus.WithError(err).Fatalf("Invalid --endpoint URL %q.", ep)
		}
	}

	githubClient := github.NewClient(secretAgent.GetTokenGenerator(*githubTokenFile), githubEndpoint.Strings()...)
	if *dryRun {
		githubClient = github.NewDryRunClient(secretAgent.GetTokenGenerator(*githubTokenFile), githubEndpoint.Strings()...)
	}

	gitClient, err := git.NewClient()
	if err != nil {
		log.WithError(err).Fatal("Error getting git client.")
	}

	// The bot name is used to determine to what fork we can push cherry-pick branches.
	botName, err := githubClient.BotName()
	if err != nil {
		log.WithError(err).Fatal("Error getting bot name.")
	}
	email, err := githubClient.Email()
	if err != nil {
		log.WithError(err).Fatal("Error getting bot e-mail.")
	}
	// The bot needs to be able to push to its own Github fork and potentially pull
	// from private repos.
	gitClient.SetCredentials(botName, secretAgent.GetTokenGenerator(*githubTokenFile))

	repos, err := githubClient.GetRepos(botName, true)
	if err != nil {
		log.WithError(err).Fatal("Error listing bot repositories.")
	}

	server := &Server{
		tokenGenerator: secretAgent.GetTokenGenerator(*webhookSecretFile),
		botName:        botName,
		email:          email,

		gc:  gitClient,
		ghc: githubClient,
		log: log,

		prowAssignments: *prowAssignments,
		allowAll:        *allowAll,

		bare:     &http.Client{},
		patchURL: "https://patch-diff.githubusercontent.com",

		repos: repos,
	}

	http.Handle("/", server)
	externalplugins.ServeExternalPluginHelp(http.DefaultServeMux, log, HelpProvider)
	logrus.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}
