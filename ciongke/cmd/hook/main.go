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
	"golang.org/x/oauth2"
	"io/ioutil"
	"log"
	"net/http"
	"os/signal"
	"regexp"
	"strconv"
	"syscall"

	"github.com/kubernetes/test-infra/ciongke/github"
	"github.com/kubernetes/test-infra/ciongke/kube"
)

var (
	port      = flag.Int("port", 8888, "Port to listen on.")
	namespace = flag.String("namespace", "default", "Namespace for all CI objects.")
	dryRun    = flag.Bool("dry-run", true, "Whether or not to avoid mutating calls to GitHub.")
	org       = flag.String("org", "kubernetes", "GitHub org to trust.")

	testPRImage = flag.String("test-pr-image", "", "Image to use for testing PRs.")

	webhookSecretFile = flag.String("hmac-secret-file", "/etc/hmac/hmac", "Path to the file containing the GitHub HMAC secret.")
	githubTokenFile   = flag.String("github-token-file", "/etc/oauth/oauth", "Path to the file containing the GitHub OAuth secret.")
)

var defaultJenkinsJobs = []JenkinsJob{
	{
		Name:      "kubernetes-pull-build-test-e2e-gce",
		Trigger:   regexp.MustCompile(`@k8s-bot (gce )?(e2e )?test this`),
		AlwaysRun: true,
		Context:   "Jenkins GCE e2e",
	},
	{
		Name:      "kubernetes-pull-build-test-e2e-gke",
		Trigger:   regexp.MustCompile(`@k8s-bot (gke )?(smoke )?(e2e )?test this`),
		AlwaysRun: true,
		Context:   "Jenkins GKE smoke e2e",
	},
	{
		Name:      "kubernetes-pull-build-test-unit-integration",
		Trigger:   regexp.MustCompile(`@k8s-bot (unit )?test this`),
		AlwaysRun: true,
		Context:   "Jenkins unit/integration",
	},
	{
		Name:      "kubernetes-pull-verify-all",
		Trigger:   regexp.MustCompile(`@k8s-bot (verify )?test this`),
		AlwaysRun: true,
		Context:   "Jenkins verification",
	},
	{
		Name:      "kubernetes-pull-build-test-federation-e2e-gce",
		Trigger:   regexp.MustCompile(`@k8s-bot federation (gce )?(e2e )?test this`),
		AlwaysRun: false,
		Context:   "Jenkins Federation GCE e2e",
	},
	{
		Name:      "kubernetes-pull-build-test-kubemark-e2e-gce",
		Trigger:   regexp.MustCompile(`@k8s-bot kubemark test this`),
		AlwaysRun: false,
		Context:   "Jenkins Jenkins Kubemark GCE e2e",
	},
}

func main() {
	flag.Parse()

	webhookSecretRaw, err := ioutil.ReadFile(*webhookSecretFile)
	if err != nil {
		log.Fatalf("Could not read webhook secret file: %s", err)
	}
	webhookSecret := bytes.TrimSpace(webhookSecretRaw)

	oauthSecretRaw, err := ioutil.ReadFile(*githubTokenFile)
	if err != nil {
		log.Fatalf("Could not read oauth secret file: %s", err)
	}
	oauthSecret := string(bytes.TrimSpace(oauthSecretRaw))

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: oauthSecret})
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	var githubClient *github.Client
	if *dryRun {
		githubClient = github.NewDryRunClient(tc)
	} else {
		githubClient = github.NewClient(tc)
	}

	kubeClient, err := kube.NewClientInCluster(*namespace)
	if err != nil {
		log.Fatalf("Error getting client: %s", err)
	}

	// Ignore SIGTERM so that we don't drop hooks when the pod is removed.
	// We'll get SIGTERM first and then SIGKILL after our graceful termination
	// deadline.
	signal.Ignore(syscall.SIGTERM)

	server := &Server{
		HMACSecret:         webhookSecret,
		PullRequestEvents:  make(chan github.PullRequestEvent),
		IssueCommentEvents: make(chan github.IssueCommentEvent),
	}

	githubAgent := &GitHubAgent{
		DryRun:       *dryRun,
		Org:          *org,
		GitHubClient: githubClient,

		JenkinsJobs: defaultJenkinsJobs,

		PullRequestEvents:  server.PullRequestEvents,
		IssueCommentEvents: server.IssueCommentEvents,

		BuildRequests: make(chan BuildRequest),
	}
	githubAgent.Start()

	kubeAgent := &KubeAgent{
		DryRun:      *dryRun,
		TestPRImage: *testPRImage,
		KubeClient:  kubeClient,
		Namespace:   *namespace,

		BuildRequests: githubAgent.BuildRequests,
	}
	kubeAgent.Start()

	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), server))
}
