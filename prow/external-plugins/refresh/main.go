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

// Refresh retries GitHub status updates for stale PR statuses.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/interrupts"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/pjutil"

	"k8s.io/test-infra/pkg/flagutil"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/config/secret"
	prowflagutil "k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/pluginhelp/externalplugins"
)

type options struct {
	port int

	configPath             string
	dryRun                 bool
	github                 prowflagutil.GitHubOptions
	instrumentationOptions prowflagutil.InstrumentationOptions
	prowURL                string

	webhookSecretFile string
}

func (o *options) Validate() error {
	for _, group := range []flagutil.OptionGroup{&o.github} {
		if err := group.Validate(o.dryRun); err != nil {
			return err
		}
	}

	if _, err := url.ParseRequestURI(o.prowURL); err != nil {
		return fmt.Errorf("invalid -prow-url URI: %q", o.prowURL)
	}

	return nil
}

func gatherOptions() options {
	o := options{}
	fs := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	fs.IntVar(&o.port, "port", 8888, "Port to listen on.")
	fs.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	fs.BoolVar(&o.dryRun, "dry-run", true, "Dry run for testing. Uses API tokens but does not mutate.")
	fs.StringVar(&o.webhookSecretFile, "hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	fs.StringVar(&o.prowURL, "prow-url", "", "Prow frontend URL.")
	for _, group := range []flagutil.OptionGroup{&o.github, &o.instrumentationOptions} {
		group.AddFlags(fs)
	}
	fs.Parse(os.Args[1:])
	return o
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.Fatalf("Invalid options: %v", err)
	}

	logrusutil.ComponentInit()
	log := logrus.StandardLogger().WithField("plugin", "refresh")

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, ""); err != nil {
		log.WithError(err).Fatal("Error starting config agent.")
	}

	secretAgent := &secret.Agent{}
	if err := secretAgent.Start([]string{o.github.TokenPath, o.webhookSecretFile}); err != nil {
		logrus.WithError(err).Fatal("Error starting secrets agent.")
	}

	githubClient, err := o.github.GitHubClient(secretAgent, o.dryRun)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting GitHub client.")
	}

	serv := &server{
		tokenGenerator: secretAgent.GetTokenGenerator(o.webhookSecretFile),
		prowURL:        o.prowURL,
		configAgent:    configAgent,
		ghc:            githubClient,
		log:            log,
	}

	health := pjutil.NewHealthOnPort(o.instrumentationOptions.HealthPort)
	health.ServeReady()

	mux := http.NewServeMux()
	mux.Handle("/", serv)
	externalplugins.ServeExternalPluginHelp(mux, log, helpProvider)
	httpServer := &http.Server{Addr: ":" + strconv.Itoa(o.port), Handler: mux}
	defer interrupts.WaitForGracefulShutdown()
	interrupts.ListenAndServe(httpServer, 5*time.Second)
}
