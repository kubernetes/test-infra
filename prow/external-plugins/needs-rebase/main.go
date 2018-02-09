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
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/external-plugins/needs-rebase/plugin"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/hook"
	"k8s.io/test-infra/prow/pluginhelp/externalplugins"
	"k8s.io/test-infra/prow/plugins"
)

var (
	port              = flag.Int("port", 8888, "Port to listen on.")
	dryRun            = flag.Bool("dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	pluginConfig      = flag.String("plugin-config", "/etc/plugins/plugins", "Path to plugin config file.")
	githubEndpoint    = flag.String("github-endpoint", "https://api.github.com", "GitHub's API endpoint.")
	githubTokenFile   = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth secret.")
	webhookSecretFile = flag.String("hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	updatePeriod      = flag.Duration("update-period", time.Hour*24, "Period duration for periodic scans of all PRs.")
)

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

	webhookSecretRaw, err := ioutil.ReadFile(*webhookSecretFile)
	if err != nil {
		log.WithError(err).Fatal("Could not read webhook secret file.")
	}
	webhookSecret := bytes.TrimSpace(webhookSecretRaw)

	oauthSecretRaw, err := ioutil.ReadFile(*githubTokenFile)
	if err != nil {
		log.WithError(err).Fatal("Could not read oauth secret file.")
	}
	oauthSecret := string(bytes.TrimSpace(oauthSecretRaw))

	_, err = url.Parse(*githubEndpoint)
	if err != nil {
		log.WithError(err).Fatal("Must specify a valid --github-endpoint URL.")
	}

	pa := &plugins.PluginAgent{}
	if err := pa.Start(*pluginConfig); err != nil {
		log.WithError(err).Fatalf("Error loading plugin config from %q.", *pluginConfig)
	}

	githubClient := github.NewClient(oauthSecret, *githubEndpoint)
	if *dryRun {
		githubClient = github.NewDryRunClient(oauthSecret, *githubEndpoint)
	}
	githubClient.Throttle(360, 360)

	server := &Server{
		hmacSecret: webhookSecret,

		ghc: githubClient,
		log: log,
	}

	go periodicUpdate(log, pa, githubClient, *updatePeriod)

	http.Handle("/", server)
	externalplugins.ServeExternalPluginHelp(http.DefaultServeMux, log, plugin.HelpProvider)
	logrus.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}

// Server implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type Server struct {
	hmacSecret []byte

	ghc *github.Client
	log *logrus.Entry
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO: Move webhook handling logic out of hook binary so that we don't have to import all
	// plugins just to validate the webhook.
	eventType, eventGUID, payload, ok := hook.ValidateWebhook(w, r, s.hmacSecret)
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
