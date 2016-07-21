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
	"flag"
	"fmt"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/kubernetes/test-infra/ciongke/cmdlog"
	"github.com/kubernetes/test-infra/ciongke/gcs"
	"github.com/kubernetes/test-infra/ciongke/github"
	"github.com/kubernetes/test-infra/ciongke/kube"
)

var (
	repoURL   = flag.String("repo-url", "", "URL of the repo to test.")
	repoName  = flag.String("repo-name", "", "Name of the repo to test.")
	pr        = flag.Int("pr", 0, "Pull request to test.")
	branch    = flag.String("branch", "", "Target branch.")
	head      = flag.String("head", "", "Head SHA.")
	workspace = flag.String("workspace", "/workspace", "Where to checkout the repo.")
	namespace = flag.String("namespace", "default", "Namespace for all CI objects.")
	dryRun    = flag.Bool("dry-run", true, "Whether or not to avoid mutating calls to GitHub.")

	sourceBucket = flag.String("source-bucket", "", "Bucket for source tars.")

	githubTokenFile = flag.String("github-token-file", "/etc/oauth/oauth", "Path to the file containing the GitHub OAuth secret.")
)

type testDescription struct {
	Name  string `yaml:"name"`
	Image string `yaml:"image"`
}

type testClient struct {
	RepoURL  string
	RepoName string
	PRNumber int
	Branch   string
	Head     string

	Workspace    string
	SourceBucket string

	GitHubClient github.Client
	KubeClient   kube.Client
	GCSClient    gcs.Client
	ExecCommand  func(name string, arg ...string) *exec.Cmd
}

func main() {
	flag.Parse()

	oauthSecretRaw, err := ioutil.ReadFile(*githubTokenFile)
	if err != nil {
		log.Printf("Could not read oauth secret file: %s", err)
		return
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
		log.Printf("Error getting Kubernetes client: %s", err)
		return
	}

	gcsClient, err := gcs.NewClient()
	if err != nil {
		log.Printf("Error getting GCS client: %s", err)
		return
	}
	client := &testClient{
		RepoURL:  *repoURL,
		RepoName: *repoName,
		PRNumber: *pr,
		Branch:   *branch,
		Head:     *head,

		Workspace:    *workspace,
		SourceBucket: *sourceBucket,

		GitHubClient: githubClient,
		KubeClient:   kubeClient,
		GCSClient:    gcsClient,
		ExecCommand:  exec.Command,
	}
	if err := client.TestPR(); err != nil {
		log.Printf("Error testing PR: %s", err)
	}
}

func (c *testClient) TestPR() error {
	mergeable, err := c.checkoutPR()
	if err != nil {
		return fmt.Errorf("error checking out git repo: %s", err)
	}
	if !mergeable {
		return fmt.Errorf("needs rebase")
	}

	if err = c.uploadSource(); err != nil {
		return fmt.Errorf("error uploading source: %s", err)
	}

	if err = c.startTests(); err != nil {
		return fmt.Errorf("error starting tests: %s", err)
	}

	return nil
}

// checkoutPR does the checkout and returns whether or not the PR can be merged.
func (c *testClient) checkoutPR() (bool, error) {
	clonePath := filepath.Join(c.Workspace, c.RepoName)
	cloneCommand := c.ExecCommand("git", "clone", "--no-checkout", c.RepoURL, clonePath)
	checkoutCommand := c.ExecCommand("git", "checkout", c.Branch)
	fetchCommand := c.ExecCommand("git", "fetch", "origin", fmt.Sprintf("pull/%d/head", c.PRNumber))
	mergeCommand := c.ExecCommand("git", "merge", c.Head, "--no-edit")
	if err := cmdlog.RunWithLogs(cloneCommand); err != nil {
		return false, err
	}
	checkoutCommand.Dir = clonePath
	if err := cmdlog.RunWithLogs(checkoutCommand); err != nil {
		return false, err
	}
	fetchCommand.Dir = clonePath
	if err := cmdlog.RunWithLogs(fetchCommand); err != nil {
		return false, err
	}
	mergeCommand.Dir = clonePath
	if err := cmdlog.RunWithLogs(mergeCommand); err != nil {
		return false, nil
	}
	return true, nil
}

// uploadSource tars and uploads the repo to GCS.
func (c *testClient) uploadSource() error {
	tarName := c.Head + ".tar.gz"
	sourcePath := filepath.Join(c.Workspace, tarName)
	tar := c.ExecCommand("tar", "czf", sourcePath, c.RepoName)
	tar.Dir = c.Workspace
	if err := cmdlog.RunWithLogs(tar); err != nil {
		return fmt.Errorf("tar failed: %s", err)
	}
	tarFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("could not open tar: %s", err)
	}
	defer tarFile.Close()
	if err := c.GCSClient.Upload(tarFile, c.SourceBucket, tarName); err != nil {
		return fmt.Errorf("source upload failed: %s", err)
	}
	return nil
}

// startTests starts the tests in the tests YAML file within the repo.
func (c *testClient) startTests() error {
	testPath := filepath.Join(c.Workspace, c.RepoName, ".test.yml")
	// If .test.yml doesn't exist, just quit here.
	if _, err := os.Stat(testPath); os.IsNotExist(err) {
		return nil
	}
	b, err := ioutil.ReadFile(testPath)
	if err != nil {
		return err
	}
	var tests []testDescription
	if err := yaml.Unmarshal(b, &tests); err != nil {
		return err
	}
	for _, test := range tests {
		log.Printf("TODO: start %s (%s)", test.Name, test.Image)
	}
	return nil
}
