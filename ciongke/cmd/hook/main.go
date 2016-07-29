/*
Copyright 2016 The Kubernetes Authors All rights reserved.

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
	"golang.org/x/oauth2"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"

	"github.com/kubernetes/test-infra/ciongke/github"
	"github.com/kubernetes/test-infra/ciongke/kube"
)

var (
	port      = flag.Int("port", 8888, "Port to listen on.")
	namespace = flag.String("namespace", "default", "Namespace for all CI objects.")
	org       = flag.String("org", "", "GitHub org to trust.")
	team      = flag.Int("team", 0, "GitHub team to trust.")
	dryRun    = flag.Bool("dry-run", true, "Whether or not to avoid mutating calls to GitHub.")

	testPRImage  = flag.String("test-pr-image", "", "Image to use for testing PRs.")
	sourceBucket = flag.String("source-bucket", "", "Bucket to store source tars in.")

	webhookSecretFile = flag.String("hmac-secret-file", "/etc/hmac/hmac", "Path to the file containing the GitHub HMAC secret.")
	githubTokenFile   = flag.String("github-token-file", "/etc/oauth/oauth", "Path to the file containing the GitHub OAuth secret.")
)

const (
	jobDeadlineSeconds = 60 * 60 * 10
)

// Server implements http.Handler.
type Server struct {
	Events chan Event

	Port         int
	Org          string
	Team         int
	GitHubClient github.Client
	HMACSecret   []byte
	DryRun       bool

	TestPRImage  string
	SourceBucket string

	KubeClient kube.Client
	Namespace  string
}

// Event is simply the GitHub event type and the JSON payload.
type Event struct {
	Type    string
	Payload []byte
}

func main() {
	flag.Parse()

	webhookSecretRaw, err := ioutil.ReadFile(*webhookSecretFile)
	if err != nil {
		log.Fatalf("Could not read webhook secret file: %s", err)
	}
	webhookSecret := bytes.TrimSpace(webhookSecretRaw)

	// TODO: Watch this file so that we don't need to manually restart when
	// we update the token.
	oauthSecretRaw, err := ioutil.ReadFile(*githubTokenFile)
	if err != nil {
		log.Fatalf("Could not read oauth secret file: %s", err)
	}
	oauthSecret := string(bytes.TrimSpace(oauthSecretRaw))

	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: oauthSecret})
	tc := oauth2.NewClient(oauth2.NoContext, ts)
	var githubClient github.Client
	if *dryRun {
		githubClient = github.NewDryRunClient(tc)
	} else {
		githubClient = github.NewClient(tc)
	}

	kubeClient, err := kube.NewClientInCluster(*namespace)
	if err != nil {
		log.Fatalf("Error getting client: %s", err)
	}

	s := &Server{
		Events: make(chan Event),

		Port:         *port,
		Org:          *org,
		Team:         *team,
		GitHubClient: githubClient,
		HMACSecret:   webhookSecret,
		DryRun:       *dryRun,

		TestPRImage:  *testPRImage,
		SourceBucket: *sourceBucket,

		KubeClient: kubeClient,
		Namespace:  *namespace,
	}
	go func() {
		for event := range s.Events {
			go s.handleEvent(event)
		}
	}()
	if err := http.ListenAndServe(":"+strconv.Itoa(s.Port), s); err != nil {
		log.Fatalf("ListenAndServe returned error: %s", err)
	}
}

// ServeHTTP validates an incoming webhook and puts it into the event channel.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	// Header checks: It must be a POST with an event type and a signature.
	if r.Method != http.MethodPost {
		http.Error(w, "405 Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		http.Error(w, "400 Bad Request: Missing X-GitHub-Event Header", http.StatusBadRequest)
		return
	}
	sig := r.Header.Get("X-Hub-Signature")
	if sig == "" {
		http.Error(w, "403 Forbidden: Missing X-Hub-Signature", http.StatusForbidden)
		return
	}

	// Validate the payload with our HMAC secret.
	payload, err := github.ValidatePayload(r, s.HMACSecret)
	if err != nil {
		http.Error(w, "403 Forbidden: Invalid X-Hub-Signature", http.StatusForbidden)
		return
	}
	fmt.Fprint(w, "Event received. Have a nice day.")
	go func() {
		s.Events <- Event{eventType, payload}
	}()
}

// handleEvent unmarshals the payload and dispatches to the correct handler.
func (s *Server) handleEvent(e Event) {
	switch e.Type {
	case "pull_request":
		var pr github.PullRequestEvent
		if err := json.Unmarshal(e.Payload, &pr); err != nil {
			log.Printf("Could not unmarshal pull request event payload: %s", err)
			return
		}
		if err := s.handlePullRequestEvent(pr); err != nil {
			log.Printf("Error handling pull request: %s", err)
			return
		}
	}
}

// handlePullRequestEvent decides what to do with PullRequestEvents.
func (s *Server) handlePullRequestEvent(pr github.PullRequestEvent) error {
	switch pr.Action {
	case "opened", "reopened", "synchronize":
		valid, err := s.validAuthor(pr.PullRequest.User.Login)
		if err != nil {
			return fmt.Errorf("could not validate author: %s", err)
		} else if valid {
			if err := s.buildPR(pr.PullRequest); err != nil {
				return fmt.Errorf("could not build PR: %s", err)
			}
		} else if pr.Action == "opened" {
			// TODO: Comment asking them to join the org.
		}
	case "closed":
		if err := s.deletePR(pr.PullRequest.Base.Repo.Name, pr.Number); err != nil {
			return fmt.Errorf("could not delete old PR: %s", err)
		}
	}
	return nil
}

// validAuthor checks if author is in the org or the team.
func (s *Server) validAuthor(author string) (bool, error) {
	orgMember, err := s.GitHubClient.IsMember(s.Org, author)
	if err != nil {
		return false, err
	} else if orgMember {
		return true, nil
	}
	return s.GitHubClient.IsTeamMember(s.Team, author)
}

// buildPR deletes any jobs building the PR and then starts a new one.
func (s *Server) buildPR(pr github.PullRequest) error {
	name := fmt.Sprintf("%s-pr-%d-%s", pr.Base.Repo.Name, pr.Number, pr.Head.SHA[:8])
	job := kube.Job{
		Metadata: kube.ObjectMeta{
			Name:      name,
			Namespace: s.Namespace,
			Labels: map[string]string{
				"repo": pr.Base.Repo.Name,
				"pr":   strconv.Itoa(pr.Number),
			},
		},
		Spec: kube.JobSpec{
			ActiveDeadlineSeconds: jobDeadlineSeconds,
			Template: kube.PodTemplateSpec{
				Spec: kube.PodSpec{
					RestartPolicy: "Never",
					Containers: []kube.Container{
						{
							Name:  "test-pr",
							Image: s.TestPRImage,
							Args: []string{
								"-repo-url=" + pr.Base.Repo.HTMLURL,
								"-repo-name=" + pr.Base.Repo.Name,
								"-pr=" + strconv.Itoa(pr.Number),
								"-branch=" + pr.Base.Ref,
								"-head=" + pr.Head.SHA,
								"-namespace=" + s.Namespace,
								"-dry-run=" + strconv.FormatBool(s.DryRun),
								"-source-bucket=" + s.SourceBucket,
								"-github-token-file=/etc/oauth/oauth",
							},
							VolumeMounts: []kube.VolumeMount{
								{
									Name:      "oauth",
									ReadOnly:  true,
									MountPath: "/etc/oauth",
								},
							},
						},
					},
					Volumes: []kube.Volume{
						{
							Name: "oauth",
							Secret: &kube.SecretSource{
								Name: "oauth-token",
							},
						},
					},
				},
			},
		},
	}
	if err := s.deletePR(pr.Base.Repo.Name, pr.Number); err != nil {
		return err
	}
	log.Printf("Starting build for PR #%d\n", pr.Number)
	if _, err := s.KubeClient.CreateJob(job); err != nil {
		return err
	}
	return nil
}

// deletePR attempts to delete the jobs for the given PR as well as their pods.
func (s *Server) deletePR(repo string, pr int) error {
	jobs, err := s.KubeClient.ListJobs(map[string]string{
		"repo": repo,
		"pr":   strconv.Itoa(pr),
	})
	if err != nil {
		return err
	}
	for _, job := range jobs {
		log.Printf("Deleting job %s", job.Metadata.Name)
		if err := s.KubeClient.DeleteJob(job.Metadata.Name); err != nil {
			return err
		}
		pods, err := s.KubeClient.ListPods(map[string]string{
			"job-name": job.Metadata.Name,
		})
		if err != nil {
			return err
		}
		for _, pod := range pods {
			if err = s.KubeClient.DeletePod(pod.Metadata.Name); err != nil {
				return err
			}
		}
	}
	return nil
}
