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
	"io/ioutil"
	"net/url"
	"time"

	"github.com/sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/git"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/tide"
)

var (
	dryRun  = flag.Bool("dry-run", true, "Whether to mutate any real-world state.")
	runOnce = flag.Bool("run-once", false, "If true, run only once then quit.")

	configPath = flag.String("config-path", "/etc/config/config", "Path to config.yaml.")
	cluster    = flag.String("cluster", "", "Path to kube.Cluster YAML file. If empty, uses the local cluster.")

	_               = flag.String("github-bot-name", "", "Deprecated.")
	githubEndpoint  = flag.String("github-endpoint", "https://api.github.com", "GitHub's API endpoint.")
	githubTokenFile = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth token.")
)

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})

	configAgent := &config.Agent{}
	if err := configAgent.Start(*configPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	oauthSecretRaw, err := ioutil.ReadFile(*githubTokenFile)
	if err != nil {
		logrus.WithError(err).Fatalf("Could not read oauth secret file.")
	}
	oauthSecret := string(bytes.TrimSpace(oauthSecretRaw))

	_, err = url.Parse(*githubEndpoint)
	if err != nil {
		logrus.WithError(err).Fatalf("Must specify a valid --github-endpoint URL.")
	}

	ghc := github.NewClient(oauthSecret, *githubEndpoint)

	var kc *kube.Client
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

	gc, err := git.NewClient()
	if err != nil {
		logrus.WithError(err).Fatal("Error getting git client.")
	}
	defer gc.Clean()

	logger := logrus.StandardLogger()
	ghc.Logger = logger.WithField("client", "github")
	kc.Logger = logger.WithField("client", "kube")
	gc.Logger = logger.WithField("client", "git")

	c := tide.NewController(ghc, kc, configAgent, gc)
	c.Logger = logger.WithField("controller", "tide")
	c.DryRun = *dryRun

	sync(c)
	if *runOnce {
		return
	}
	for range time.Tick(time.Minute) {
		sync(c)
	}
}

func sync(c *tide.Controller) {
	start := time.Now()
	if err := c.Sync(); err != nil {
		logrus.WithError(err).Error("Error syncing.")
	}
	logrus.Infof("Sync time: %v", time.Since(start))
}
