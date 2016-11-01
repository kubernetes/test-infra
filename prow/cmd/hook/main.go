/*
Copyright 2016 The Kubernetes Authors.

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
	"net/http"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/Sirupsen/logrus"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/jobs"
	"k8s.io/test-infra/prow/kube"
)

var (
	port      = flag.Int("port", 8888, "Port to listen on.")
	namespace = flag.String("namespace", "default", "Namespace for all CI objects.")
	dryRun    = flag.Bool("dry-run", true, "Whether or not to avoid mutating calls to GitHub.")
	org       = flag.String("org", "kubernetes", "GitHub org to trust.")

	jobConfig    = flag.String("job-config", "/etc/jobs/jobs", "Path to job config file.")
	pluginConfig = flag.String("plugin-config", "/etc/plugins/plugins", "Path to plugin config file.")

	lineImage = flag.String("line-image", "", "Image to use for testing PRs.")

	webhookSecretFile = flag.String("hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	githubTokenFile   = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth secret.")
)

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})

	webhookSecretRaw, err := ioutil.ReadFile(*webhookSecretFile)
	if err != nil {
		logrus.Fatalf("Could not read webhook secret file: %s", err)
	}
	webhookSecret := bytes.TrimSpace(webhookSecretRaw)

	oauthSecretRaw, err := ioutil.ReadFile(*githubTokenFile)
	if err != nil {
		logrus.Fatalf("Could not read oauth secret file: %s", err)
	}
	oauthSecret := string(bytes.TrimSpace(oauthSecretRaw))

	var githubClient *github.Client
	if *dryRun {
		githubClient = github.NewDryRunClient(oauthSecret)
	} else {
		githubClient = github.NewClient(oauthSecret)
	}

	kubeClient, err := kube.NewClientInCluster(*namespace)
	if err != nil {
		logrus.Fatalf("Error getting client: %s", err)
	}

	// Ignore SIGTERM so that we don't drop hooks when the pod is removed.
	// We'll get SIGTERM first and then SIGKILL after our graceful termination
	// deadline.
	signal.Ignore(syscall.SIGTERM)

	prc := make(chan github.PullRequestEvent)
	icc := make(chan github.IssueCommentEvent)
	brc := make(chan KubeRequest)
	drc := make(chan KubeRequest)
	server := &Server{
		HMACSecret:         webhookSecret,
		PullRequestEvents:  prc,
		IssueCommentEvents: icc,
	}

	jobAgent := &jobs.JobAgent{}
	jobAgent.Start(*jobConfig)

	pluginAgent := &PluginAgent{}
	pluginAgent.Start(*pluginConfig)

	githubAgent := &GitHubAgent{
		DryRun:       *dryRun,
		Org:          *org,
		GitHubClient: githubClient,

		JenkinsJobs: jobAgent,
		Plugins:     pluginAgent,

		PullRequestEvents:  prc,
		IssueCommentEvents: icc,

		BuildRequests:  brc,
		DeleteRequests: drc,
	}
	githubAgent.Start()

	kubeAgent := &KubeAgent{
		DryRun:     *dryRun,
		LineImage:  *lineImage,
		KubeClient: kubeClient,
		Namespace:  *namespace,

		BuildRequests:  brc,
		DeleteRequests: drc,
	}
	kubeAgent.Start()

	logrus.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), server))
}
