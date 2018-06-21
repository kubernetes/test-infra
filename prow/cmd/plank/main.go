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
	"flag"
	"fmt"
	"io/ioutil"
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

var (
	totURL = flag.String("tot-url", "", "Tot URL")

	configPath    = flag.String("config-path", "/etc/config/config.yaml", "Path to config.yaml.")
	jobConfigPath = flag.String("job-config-path", "", "Path to prow job configs.")
	cluster       = flag.String("cluster", "", "Path to kube.Cluster YAML file. If empty, uses the local cluster.")
	buildCluster  = flag.String("build-cluster", "", "Path to file containing a YAML-marshalled kube.Cluster object. If empty, uses the local cluster.")
	selector      = flag.String("label-selector", kube.EmptySelector, "Label selector to be applied in prowjobs. See https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors for constructing a label selector.")

	githubEndpoint  = flagutil.NewStrings("https://api.github.com")
	githubTokenFile = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth token.")
	dryRun          = flag.Bool("dry-run", true, "Whether or not to make mutating API calls to GitHub.")
	deckURL         = flag.String("deck-url", "", "Deck URL for read-only access to the cluster.")
)

func init() {
	flag.Var(&githubEndpoint, "github-endpoint", "GitHub's API endpoint.")
}

func main() {
	flag.Parse()
	logrus.SetFormatter(
		logrusutil.NewDefaultFieldsFormatter(nil, logrus.Fields{"component": "plank"}),
	)

	if _, err := labels.Parse(*selector); err != nil {
		logrus.WithError(err).Fatal("Error parsing label selector.")
	}

	configAgent := &config.Agent{}
	if err := configAgent.Start(*configPath, *jobConfigPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	var oauthSecret string
	var err error
	if *githubTokenFile != "" {
		oauthSecretRaw, err := ioutil.ReadFile(*githubTokenFile)
		if err != nil {
			logrus.WithError(err).Fatalf("Could not read oauth secret file.")
		}

		for _, ep := range githubEndpoint.Strings() {
			_, err = url.Parse(ep)
			if err != nil {
				logrus.WithError(err).Fatalf("Invalid --endpoint URL %q.", ep)
			}
		}

		oauthSecret = string(bytes.TrimSpace(oauthSecretRaw))
	}

	var ghc *github.Client
	var kc *kube.Client
	var pkcs map[string]*kube.Client
	if *dryRun {
		if oauthSecret != "" {
			ghc = github.NewDryRunClient(oauthSecret, githubEndpoint.Strings()...)
		}
		kc = kube.NewFakeClient(*deckURL)
		pkcs = map[string]*kube.Client{kube.DefaultClusterAlias: kc}
	} else {
		if oauthSecret != "" {
			ghc = github.NewClient(oauthSecret, githubEndpoint.Strings()...)
		}
		if *cluster == "" {
			kc, err = kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
			if err != nil {
				logrus.WithError(err).Fatal("Error getting kube client.")
			}
		} else {
			kc, err = kube.NewClientFromFile(*cluster, configAgent.Config().ProwJobNamespace)
			if err != nil {
				logrus.WithError(err).Fatal("Error getting kube client.")
			}
		}
		if *buildCluster == "" {
			pkc, err := kube.NewClientInCluster(configAgent.Config().PodNamespace)
			if err != nil {
				logrus.WithError(err).Fatal("Error getting kube client.")
			}
			pkcs = map[string]*kube.Client{kube.DefaultClusterAlias: pkc}
		} else {
			pkcs, err = kube.ClientMapFromFile(*buildCluster, configAgent.Config().PodNamespace)
			if err != nil {
				logrus.WithError(err).Fatal("Error getting kube client to build cluster.")
			}
		}
	}

	c, err := plank.NewController(kc, pkcs, ghc, nil, configAgent, *totURL, *selector)
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
