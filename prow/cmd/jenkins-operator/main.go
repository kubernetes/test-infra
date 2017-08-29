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

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/jenkins"
	"k8s.io/test-infra/prow/kube"
)

var (
	configPath = flag.String("config-path", "/etc/config/config", "Path to config.yaml.")

	jenkinsURL       = flag.String("jenkins-url", "http://jenkins-proxy", "Jenkins URL")
	jenkinsUserName  = flag.String("jenkins-user", "jenkins-trigger", "Jenkins username")
	jenkinsTokenFile = flag.String("jenkins-token-file", "/etc/jenkins/jenkins", "Path to the file containing the Jenkins API token.")

	_               = flag.String("github-bot-name", "", "Deprecated.")
	githubEndpoint  = flag.String("github-endpoint", "https://api.github.com", "GitHub's API endpoint.")
	githubTokenFile = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth token.")
	dryRun          = flag.Bool("dry-run", true, "Whether or not to make mutating API calls to GitHub.")
)

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})

	configAgent := &config.Agent{}
	if err := configAgent.Start(*configPath); err != nil {
		logrus.WithError(err).Fatal("Error starting config agent.")
	}

	kc, err := kube.NewClientInCluster(configAgent.Config().ProwJobNamespace)
	if err != nil {
		logrus.WithError(err).Fatal("Error getting kube client.")
	}

	jenkinsSecretRaw, err := ioutil.ReadFile(*jenkinsTokenFile)
	if err != nil {
		logrus.WithError(err).Fatalf("Could not read Jenkins token file.")
	}
	jenkinsToken := string(bytes.TrimSpace(jenkinsSecretRaw))
	jc := jenkins.NewClient(*jenkinsURL, *jenkinsUserName, jenkinsToken)

	oauthSecretRaw, err := ioutil.ReadFile(*githubTokenFile)
	if err != nil {
		logrus.WithError(err).Fatalf("Could not read Github oauth secret file.")
	}
	oauthSecret := string(bytes.TrimSpace(oauthSecretRaw))

	_, err = url.Parse(*githubEndpoint)
	if err != nil {
		logrus.WithError(err).Fatal("Must specify a valid --github-endpoint URL.")
	}

	var ghc *github.Client
	if *dryRun {
		ghc = github.NewDryRunClient(oauthSecret, *githubEndpoint)
	} else {
		ghc = github.NewClient(oauthSecret, *githubEndpoint)
	}

	c, err := jenkins.NewController(kc, jc, ghc, configAgent)
	if err != nil {
		logrus.WithError(err).Fatal("Error creating jenkins controller.")
	}
	for range time.Tick(30 * time.Second) {
		start := time.Now()
		if err := c.Sync(); err != nil {
			logrus.WithError(err).Error("Error syncing.")
		}
		logrus.Infof("Sync time: %v", time.Since(start))
	}
}
