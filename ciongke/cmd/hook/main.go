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
	"github.com/Sirupsen/logrus"
	"golang.org/x/oauth2"
	"io/ioutil"
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

	webhookSecretFile = flag.String("hmac-secret-file", "/etc/webhook/hmac", "Path to the file containing the GitHub HMAC secret.")
	githubTokenFile   = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth secret.")
)

var defaultJenkinsJobs = map[string][]JenkinsJob{
	"kubernetes/kubernetes": {
		{
			Name:      "kubernetes-pull-build-test-e2e-gke",
			Trigger:   regexp.MustCompile(`@k8s-bot (gke )?(e2e )?test this`),
			AlwaysRun: true,
			Context:   "Jenkins GKE smoke e2e",
		},
		{
			Name:    "kubernetes-pull-build-test-federation-e2e-gce",
			Trigger: regexp.MustCompile(`@k8s-bot federation (gce )?(e2e )?test this`),
			Context: "Jenkins Federation GCE e2e",
		},
		{
			Name:    "kubernetes-pull-build-test-gci-federation-e2e-gce",
			Trigger: regexp.MustCompile(`@k8s-bot federation (gci )?(gce )?(e2e )?test this`),
			Context: "Jenkins GCI Federation GCE e2e",
		},
		{
			Name:      "kubernetes-pull-build-test-kubemark-e2e-gce",
			Trigger:   regexp.MustCompile(`@k8s-bot kubemark (e2e )?test this`),
			AlwaysRun: true,
			Context:   "Jenkins Kubemark GCE e2e",
		},
		{
			Name:    "kubernetes-pull-build-test-gci-kubemark-e2e-gce",
			Trigger: regexp.MustCompile(`@k8s-bot kubemark (gci )?(e2e )?test this`),
			Context: "Jenkins GCI Kubemark GCE e2e",
		},
		{
			Name:    "fejta-pull-unit",
			Trigger: regexp.MustCompile(`@fejta do a little dance`),
			Context: "Jenkins fejta unit",
		},
		{
			Name:    "fejta-pull-e2e",
			Trigger: regexp.MustCompile(`@fejta do a little dance`),
			Context: "Jenkins fejta gce",
		},
	},
	"kubernetes/test-infra": {
		{
			Name:             "testinfra-pull-gotest",
			Trigger:          regexp.MustCompile(`@k8s-bot (go )?test this`),
			AlwaysRun:        true,
			Context:          "Jenkins go test",
			CommentOnFailure: true,
			RerunCommand:     "@k8s-bot go test this",
		},
	},
}

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

	githubAgent := &GitHubAgent{
		DryRun:       *dryRun,
		Org:          *org,
		GitHubClient: githubClient,

		JenkinsJobs: defaultJenkinsJobs,

		PullRequestEvents:  prc,
		IssueCommentEvents: icc,

		BuildRequests:  brc,
		DeleteRequests: drc,
	}
	githubAgent.Start()

	kubeAgent := &KubeAgent{
		DryRun:      *dryRun,
		TestPRImage: *testPRImage,
		KubeClient:  kubeClient,
		Namespace:   *namespace,

		BuildRequests:  brc,
		DeleteRequests: drc,
	}
	kubeAgent.Start()

	logrus.Fatal(http.ListenAndServe(":"+strconv.Itoa(*port), server))
}
