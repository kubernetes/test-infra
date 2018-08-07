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
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/external-plugins/needs-rebase/plugin"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pluginhelp/externalplugins"
	"k8s.io/test-infra/prow/plugins"
)

var (
	port              = flag.Int("port", 8888, "Port to listen on.")
	dryRun            = flag.Bool("dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	pluginConfig      = flag.String("plugin-config", "/etc/plugins/plugins.yaml", "Path to plugin config file.")
	githubEndpoint    = flagutil.NewStrings("https://api.github.com")
	githubTokenFile   = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth secret.")
	webhookSecretFile = flag.String("hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	updatePeriod      = flag.Duration("update-period", time.Hour*24, "Period duration for periodic scans of all PRs.")
)

func init() {
	flag.Var(&githubEndpoint, "github-endpoint", "GitHub's API endpoint.")
}

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})
	// TODO: Use global option from the prow config.
	logrus.SetLevel(logrus.InfoLevel)
	log := logrus.StandardLogger().WithField("plugin", "needs-rebase")

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

	pa := &plugins.PluginAgent{}
	if err := pa.Start(*pluginConfig); err != nil {
		log.WithError(err).Fatalf("Error loading plugin config from %q.", *pluginConfig)
	}

	githubClient := github.NewClient(secretAgent.GetTokenGenerator(*githubTokenFile), githubEndpoint.Strings()...)
	if *dryRun {
		githubClient = github.NewDryRunClient(secretAgent.GetTokenGenerator(*githubTokenFile), githubEndpoint.Strings()...)
	}
	githubClient.Throttle(360, 360)

	server := &Server{
		tokenGenerator: secretAgent.GetTokenGenerator(*webhookSecretFile),
		ghc:            githubClient,
		log:            log,
	}

	go periodicUpdate(log, pa, githubClient, *updatePeriod)

	http.Handle("/", server)
	externalplugins.ServeExternalPluginHelp(http.DefaultServeMux, log, plugin.HelpProvider)
	logrus.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}

// Server implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type Server struct {
	tokenGenerator func() []byte
	ghc            *github.Client
	log            *logrus.Entry
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	eventType, eventGUID, payload, ok := github.ValidateWebhook(w, r, s.tokenGenerator())
	if !ok {
		return
	}
	fmt.Fprint(w, "Event received. Have a nice day.")

	if err := s.handleEvent(eventType, eventGUID, payload); err != nil {
		logrus.WithError(err).Error("Error parsing event.")
	}
}

func (s *Server) handleEvent(eventType, eventGUID string, payload []byte) error {
	l := s.log.WithFields(
		logrus.Fields{
			"event-type":     eventType,
			github.EventGUID: eventGUID,
		},
	)
	switch eventType {
	case "pull_request":
		var pre github.PullRequestEvent
		if err := json.Unmarshal(payload, &pre); err != nil {
			return err
		}
		go func() {
			if err := plugin.HandleEvent(l, s.ghc, &pre); err != nil {
				l.Info("Error handling event.")
			}
		}()
	default:
		s.log.Debugf("received an event of type %q but didn't ask for it", eventType)
	}
	return nil
}

func periodicUpdate(log *logrus.Entry, pa *plugins.PluginAgent, ghc *github.Client, period time.Duration) {
	update := func() {
		start := time.Now()
		if err := plugin.HandleAll(log, ghc, pa.Config()); err != nil {
			log.WithError(err).Error("Error during periodic update of all PRs.")
		}
		log.WithField("duration", fmt.Sprintf("%v", time.Since(start))).Info("Periodic update complete.")
	}

	update()
	for range time.Tick(period) {
		update()
	}
}
