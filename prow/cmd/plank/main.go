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
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/flagutil"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/logrusutil"
	"k8s.io/test-infra/prow/metrics"
	"k8s.io/test-infra/prow/plank"
)

type options struct {
	totURL string

	configPath    string
	jobConfigPath string
	cluster       string
	buildCluster  string
	selector      string

	githubEndpoint  flagutil.Strings
	githubTokenFile string
	dryRun          bool
	deckURL         string
}

func gatherOptions() options {
	o := options{
		githubEndpoint: flagutil.NewStrings("https://api.github.com"),
	}

	flag.StringVar(&o.totURL, "tot-url", "", "Tot URL")

	flag.StringVar(&o.configPath, "config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	flag.StringVar(&o.jobConfigPath, "job-config-path", "", "Path to prow job configs.")
	flag.StringVar(&o.cluster, "cluster", "", "Path to kube.Cluster YAML file. If empty, uses the local cluster.")
	flag.StringVar(&o.buildCluster, "build-cluster", "", "Path to file containing a YAML-marshalled kube.Cluster object. If empty, uses the local cluster.")
	flag.StringVar(&o.selector, "label-selector", kube.EmptySelector, "Label selector to be applied in prowjobs. See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors for constructing a label selector.")

	flag.Var(&o.githubEndpoint, "github-endpoint", "GitHub's API endpoint.")
	flag.StringVar(&o.githubTokenFile, "github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth token.")
	flag.BoolVar(&o.dryRun, "dry-run", true, "Whether or not to make mutating API calls to GitHub.")
	flag.StringVar(&o.deckURL, "deck-url", "", "Deck URL for read-only access to the cluster.")
	flag.Parse()
	return o
}

func (o *options) Validate() error {
	if _, err := labels.Parse(o.selector); err != nil {
		return fmt.Errorf("parse label selector: %v", err)
	}

	return nil
}

func main() {
	o := gatherOptions()
	if err := o.Validate(); err != nil {
		logrus.WithError(err).Fatal("Invalid options")
	}

	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "plank"}),
	)

	configAgent := &config.Agent{}
	if err := configAgent.Start(o.configPath, o.jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	var err error
	// Check if github endpoint is a valid url.
	for _, ep := range o.githubEndpoint.Strings() {
		_, err = url.ParseRequestURI(ep)
		if err != nil {
			logrus.WithError(err).Fatalf("Invalid --endpoint URL %q.", ep)
		}
	}

	var ghc plank.GitHubClient
	if o.githubTokenFile != "" {
		secretAgent := &config.SecretAgent{}
		if err := secretAgent.Start([]string{o.githubTokenFile}); err != nil {
			logrus.WithError(err).Fatal("Error starting secrets agent.")
		}

		getSecret := func(secretPath string) func() []byte {
			return func() []byte {
				return secretAgent.GetSecret(secretPath)
			}
		}

		if o.dryRun {
			if string(getSecret(o.githubTokenFile)()) != "" {
				ghc = github.NewDryRunClient(getSecret(o.githubTokenFile),
					o.githubEndpoint.Strings()...)
			}
		} else {
			if string(getSecret(o.githubTokenFile)()) != "" {
				ghc = github.NewClient(getSecret(o.githubTokenFile),
					o.githubEndpoint.Strings()...)
			}
		}
	}

	var kc *kube.Client
	var pkcs map[string]*kube.Client
	if o.dryRun {
		kc = kube.NewFakeClient(o.deckURL)
		pkcs = map[string]*kube.Client{kube.DefaultClusterAlias: kc}
	} else {
		if o.cluster == "" {
			kc, err = kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
			if err != nil {
				logrus.WithError(err).Fatal("Error getting kube client.")
			}
		} else {
			kc, err = kube.NewClientFromFile(o.cluster, configAgent.Config().ProwJobNamespace)
			if err != nil {
				logrus.WithError(err).Fatal("Error getting kube client.")
			}
		}
		if o.buildCluster == "" {
			pkc, err := kube.NewClientInCluster(configAgent.Config().PodNamespace)
			if err != nil {
				logrus.WithError(err).Fatal("Error getting kube client.")
			}
			pkcs = map[string]*kube.Client{kube.DefaultClusterAlias: pkc}
		} else {
			pkcs, err = kube.ClientMapFromFile(o.buildCluster, configAgent.Config().PodNamespace)
			if err != nil {
				logrus.WithError(err).Fatal("Error getting kube client to build cluster.")
			}
		}
	}

	c, err := plank.NewController(kc, pkcs, ghc, nil, configAgent, o.totURL, o.selector)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating plank controller.")
	}

	// Push metrics to the configured prometheus pushgateway endpoint.
	pushGateway := configAgent.Config().PushGateway
	if pushGateway.Endpoint != "" {
		go metrics.PushMetrics("plank", pushGateway.Endpoint, pushGateway.Interval)
	}
	// serve prometheus metrics.
	go serve()
	// gather metrics for the jobs handled by plank.
	go gather(c)

	tick := time.Tick(30 * time.Second)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-tick:
			start := time.Now()
			if err := c.Sync(); err != nil {
				logrus.WithError(err).Error("Error syncing.")
			}
			logrus.WithField("duration", fmt.Sprintf("%v", time.Since(start))).Info("Synced")
		case <-sig:
			logrus.Info("Plank is shutting down...")
			return
		}
	}
}

// serve starts a http server and serves prometheus metrics.
// Meant to be called inside a goroutine.
func serve() {
	http.Handle("/metrics", promhttp.Handler())
	logrus.WithError(http.ListenAndServe(":8080", nil)).Fatal("ListenAndServe returned.")
}

// gather metrics from plank.
// Meant to be called inside a goroutine.
func gather(c *plank.Controller) {
	tick := time.Tick(30 * time.Second)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-tick:
			start := time.Now()
			c.SyncMetrics()
			logrus.WithField("metrics-duration", fmt.Sprintf("%v", time.Since(start))).Debug("Metrics synced")
		case <-sig:
			logrus.Debug("Plank gatherer is shutting down...")
			return
		}
	}
}
