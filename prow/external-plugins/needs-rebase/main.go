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
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/pjutil"

	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/config/secret"
	"k8s.io/test-infra/prow/external-plugins/needs-rebase/plugin"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/logrusutil"

	"k8s.io/test-infra/prow/pluginhelp/externalplugins"
	"k8s.io/test-infra/prow/plugins"
)

type options struct {
	port int

	pluginConfig           string
	dryRun                 bool
	github                 prowflagutil.GitHubOptions
	instrumentationOptions prowflagutil.InstrumentationOptions
	logLevel               string
	hourlyTokens           int

	updatePeriod time.Duration

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
	fs.StringVar(&o.pluginConfig, "plugin-config", "/etc/plugins/plugins.yaml", "Path to plugin config file.")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	fs.DurationVar(&o.updatePeriod, "update-period", time.Hour*24, "Period duration for periodic scans of all PRs.")
	fs.StringVar(&o.webhookSecretFile, "hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	fs.StringVar(&o.logLevel, "log-level", "debug", fmt.Sprintf("Log level is one of %v.", logrus.AllLevels))
	fs.IntVar(&o.hourlyTokens, "hourly-tokens", 360, "The number of hourly tokens need-rebase may use")

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
	log := logrus.StandardLogger().WithField("plugin", labels.NeedsRebase)

	secretAgent := &secret.Agent{}
	secrets := []string{o.webhookSecretFile}
	if o.github.TokenPath != "" {
		secrets = append(secrets, o.github.TokenPath)
	}
	if err := secretAgent.Start(secrets); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	pa := &plugins.ConfigAgent{}
	if err := pa.Start(o.pluginConfig, false); err != nil {
		log.WithError(err).Fatalf("Error loading plugin config from %q.", o.pluginConfig)
	}

	githubClient, err := o.github.GitHubClient(secretAgent, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}
	githubClient.Throttle(o.hourlyTokens, o.hourlyTokens)

	server := &Server{
		tokenGenerator: secretAgent.GetTokenGenerator(o.webhookSecretFile),
		ghc:            githubClient,
		log:            log,
	}

	defer interrupts.WaitForGracefulShutdown()

	interrupts.TickLiteral(func() {
		start := time.Now()
		if err := plugin.HandleAll(log, githubClient, pa.Config(), o.github.AppID != ""); err != nil {
			log.WithError(err).Error("Error during periodic update of all PRs.")
		}
		log.WithField("duration", fmt.Sprintf("%v", time.Since(start))).Info("Periodic update complete.")
	}, o.updatePeriod)

	health := pjutil.NewHealthOnPort(o.instrumentationOptions.HealthPort)
	health.ServeReady()

	mux := http.NewServeMux()
	mux.Handle("/", server)
	externalplugins.ServeExternalPluginHelp(mux, log, plugin.HelpProvider)
	httpServer := &http.Server{Addr: ":" + strconv.Itoa(o.port), Handler: mux}
	interrupts.ListenAndServe(httpServer, 5*time.Second)
}

// Server implements http.Handler. It validates incoming GitHub webhooks and
// then dispatches them to the appropriate plugins.
type Server struct {
	tokenGenerator func() []byte
	ghc            github.Client
	log            *logrus.Entry
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO: Move webhook handling logic out of hook binary so that we don't have to import all
	// plugins just to validate the webhook.
	eventType, eventGUID, payload, ok, _ := github.ValidateWebhook(w, r, s.tokenGenerator)
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
			if err := plugin.HandlePullRequestEvent(l, s.ghc, &pre); err != nil {
				l.WithField("event-type", eventType).WithError(err).Info("Error handling event.")
			}
		}()
	case "issue_comment":
		var ice github.IssueCommentEvent
		if err := json.Unmarshal(payload, &ice); err != nil {
			return err
		}
		go func() {
			if err := plugin.HandleIssueCommentEvent(l, s.ghc, &ice); err != nil {
				l.WithField("event-type", eventType).WithError(err).Info("Error handling event.")
			}
		}()
	default:
		s.log.Debugf("received an event of type %q but didn't ask for it", eventType)
	}
	return nil
}
