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
	"time"

	"github.com/kubernetes/test-infra/ciongke/github"
	"github.com/kubernetes/test-infra/ciongke/jenkins"
)

var (
	job       = flag.String("job-name", "", "Which Jenkins job to build.")
	context   = flag.String("context", "", "Build status context.")
	repoOwner = flag.String("repo-owner", "", "Owner of the repo.")
	repoName  = flag.String("repo-name", "", "Name of the repo to test.")
	pr        = flag.Int("pr", 0, "Pull request to test.")
	branch    = flag.String("branch", "", "Target branch.")
	commit    = flag.String("sha", "", "Head SHA of the PR.")
	dryRun    = flag.Bool("dry-run", true, "Whether or not to make mutating GitHub/Jenkins calls.")

	githubTokenFile  = flag.String("github-token-file", "/etc/github/oauth", "Path to the file containing the GitHub OAuth secret.")
	jenkinsURL       = flag.String("jenkins-url", "http://pull-jenkins-master:8080/", "Jenkins URL")
	jenkinsUserName  = flag.String("jenkins-user", "jenkins-trigger", "Jenkins username")
	jenkinsTokenFile = flag.String("jenkins-token-file", "/etc/jenkins/jenkins", "Path to the file containing the Jenkins API token.")
)

type testClient struct {
	Job     string
	Context string

	RepoOwner string
	RepoName  string
	PRNumber  int
	Branch    string
	Commit    string

	DryRun bool

	JenkinsClient *jenkins.Client
	GitHubClient  *github.Client
}

func main() {
	flag.Parse()
	logrus.SetFormatter(&logrus.JSONFormatter{})

	jenkinsSecretRaw, err := ioutil.ReadFile(*jenkinsTokenFile)
	if err != nil {
		logrus.Fatalf("Could not read token file: %s", err)
	}
	jenkinsToken := string(bytes.TrimSpace(jenkinsSecretRaw))

	var jenkinsClient *jenkins.Client
	if *dryRun {
		jenkinsClient = jenkins.NewDryRunClient(*jenkinsURL, *jenkinsUserName, jenkinsToken)
	} else {
		jenkinsClient = jenkins.NewClient(*jenkinsURL, *jenkinsUserName, jenkinsToken)
	}

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

	client := &testClient{
		Job:       *job,
		Context:   *context,
		RepoOwner: *repoOwner,
		RepoName:  *repoName,
		PRNumber:  *pr,
		Branch:    *branch,
		Commit:    *commit,
		DryRun:    *dryRun,

		JenkinsClient: jenkinsClient,
		GitHubClient:  githubClient,
	}
	if err := client.TestPR(); err != nil {
		logrus.Errorf("Error testing PR: %s", err)
		return
	}
}

func fields(c *testClient) logrus.Fields {
	return logrus.Fields{
		"job":    c.Job,
		"org":    c.RepoOwner,
		"repo":   c.RepoName,
		"pr":     c.PRNumber,
		"branch": c.Branch,
		"commit": c.Commit,
	}
}

// TestPR starts a Jenkins build and watches it, updating the GitHub status as
// necessary.
func (c *testClient) TestPR() error {
	logrus.WithFields(fields(c)).Info("Starting build.")
	b, err := c.JenkinsClient.Build(c.Job, c.PRNumber, c.Branch)
	if err != nil {
		return err
	}
	eq, err := c.JenkinsClient.Enqueued(b)
	if err != nil {
		c.tryCreateStatus(github.Error, "Error queueing build.", "")
		return err
	}
	if eq {
		c.tryCreateStatus(github.Pending, "Build queued.", "")
	}
	for eq {
		time.Sleep(10 * time.Second)
		eq, err = c.JenkinsClient.Enqueued(b)
		if err != nil {
			c.tryCreateStatus(github.Error, "Error in queue.", "")
			return err
		}
	}
	c.tryCreateStatus(github.Pending, "Build started.", "")
	for {
		result, err := c.JenkinsClient.Status(b)
		if err != nil {
			c.tryCreateStatus(github.Error, "Error waiting for build.", "")
			return err
		}
		if result.Building {
			time.Sleep(30 * time.Second)
		} else {
			if result.Success {
				c.tryCreateStatus(github.Success, "Build succeeded.", result.URL)
				break
			} else {
				c.tryCreateStatus(github.Failure, "Build failed.", result.URL)
				break
			}
		}
	}
	return nil
}

func (c *testClient) tryCreateStatus(state, desc, url string) {
	logrus.WithFields(fields(c)).Infof("Setting status to %s: %s", state, desc)
	err := c.GitHubClient.CreateStatus(c.RepoOwner, c.RepoName, c.Commit, github.Status{
		State:       state,
		Description: desc,
		Context:     c.Context,
		TargetURL:   url,
	})
	if err != nil {
		logrus.WithFields(fields(c)).Errorf("Error creating GitHub status: %s", err)
	}
}
