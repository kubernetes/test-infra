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
	"fmt"
	"golang.org/x/oauth2"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/kubernetes/test-infra/ciongke/gcs"
	"github.com/kubernetes/test-infra/ciongke/github"
)

var (
	repoOwner = flag.String("repo-owner", "", "Owner of the GitHub repository.")
	repoName  = flag.String("repo-name", "", "Name of the GitHub repository.")
	pr        = flag.Int("pr", 0, "Pull request to test.")
	head      = flag.String("head", "", "Head SHA to test.")
	testName  = flag.String("test-name", "", "Name of the test.")
	testImage = flag.String("test-image", "", "Image to run.")
	testPath  = flag.String("test-path", "/repo", "Location to mount the source volume within the container.")
	workspace = flag.String("workspace", "/workspace", "Where to run the test.")
	timeout   = flag.Duration("timeout", time.Hour, "Test timeout.")

	sourceBucket = flag.String("source-bucket", "", "Bucket for source tars.")

	githubTokenFile = flag.String("github-token-file", "/etc/oauth/oauth", "Path to the file containing the GitHub OAuth secret.")
	dryRun          = flag.Bool("dry-run", true, "Whether or not to avoid mutating calls to GitHub.")
)

const (
	statusContext = "ciongke"

	dockerEndpoint    = "unix:///var/run/docker.sock"
	dockerKillTimeout = 10
)

type gcsClient interface {
	Download(bucket, name string) ([]byte, error)
}

type testClient struct {
	RepoOwner string
	RepoName  string
	PRNumber  int
	Head      string

	TestName  string
	TestImage string
	TestPath  string
	Timeout   time.Duration

	Workspace    string
	SourceBucket string

	GCSClient    gcsClient
	DockerClient *docker.Client
	GitHubClient *github.Client
}

func main() {
	flag.Parse()

	gcsClient, err := gcs.NewClient()
	if err != nil {
		log.Printf("Error getting GCS client: %s", err)
		return
	}

	dockerClient, err := docker.NewClient(dockerEndpoint)
	if err != nil {
		log.Printf("Error getting Docker client: %s", err)
		return
	}
	// If the docker server isn't responding, start it.
	if err := dockerClient.Ping(); err != nil {
		log.Print("Starting docker daemon")
		startDaemon := exec.Command("docker", "daemon")
		if err := startDaemon.Start(); err != nil {
			log.Printf("Error starting docker daemon: %s", err)
			return
		}
		// Wait for the docker daemon to show up. This is gross, and there
		// might be a better way.
		time.Sleep(5 * time.Second)
	}
	if err := dockerClient.Ping(); err != nil {
		log.Printf("Could not start docker daemon: %s", err)
		return
	}

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

	client := &testClient{
		RepoOwner: *repoOwner,
		RepoName:  *repoName,
		PRNumber:  *pr,
		Head:      *head,
		TestName:  *testName,
		TestImage: *testImage,
		TestPath:  *testPath,
		Timeout:   *timeout,

		Workspace:    *workspace,
		SourceBucket: *sourceBucket,

		GCSClient:    gcsClient,
		DockerClient: dockerClient,
		GitHubClient: githubClient,
	}
	client.RunTest()
}

func (c *testClient) RunTest() {
	status := github.Status{
		State:       github.Pending,
		Description: c.TestName + " started",
		Context:     statusContext,
	}
	if err := c.GitHubClient.CreateStatus(c.RepoOwner, c.RepoName, c.Head, status); err != nil {
		log.Printf("Error setting github status: %s", err)
	}
	ec, err := c.runTest()
	if err != nil {
		log.Printf("Error running test: %s", err)
		status = github.Status{
			State:       github.Error,
			Description: c.TestName + " errored",
			Context:     statusContext,
		}
	} else if ec == 0 {
		log.Print("Test succeeded")
		status = github.Status{
			State:       github.Success,
			Description: c.TestName + " succeeded",
			Context:     statusContext,
		}
	} else {
		log.Printf("Test failed with exit code %d", ec)
		status = github.Status{
			State:       github.Failure,
			Description: c.TestName + " failed",
			Context:     statusContext,
		}
	}
	if err := c.GitHubClient.CreateStatus(c.RepoOwner, c.RepoName, c.Head, status); err != nil {
		log.Printf("Error setting github status: %s", err)
	}
}

// runTest gets the source and runs the test. It returns the test container's
// exit code.
func (c *testClient) runTest() (int, error) {
	r, err := c.getSource()
	if err != nil {
		return 0, fmt.Errorf("error getting source from GCS: %s", err)
	}
	exitCode, err := c.runTestContainer(r)
	if err != nil {
		return 0, fmt.Errorf("error running test container: %s", err)
	}
	return exitCode, nil
}

// getSource returns an io.Reader containing the source tar.
func (c *testClient) getSource() (io.Reader, error) {
	tarName := fmt.Sprintf("%d.tar.gz", c.PRNumber)
	b, err := c.GCSClient.Download(c.SourceBucket, tarName)
	if err != nil {
		return nil, fmt.Errorf("error downloading from GCS: %s", err)
	}
	return bytes.NewBuffer(b), nil
}

// runTestContainer creates the test container, copies in the source from
// the reader, runs it, and returns the exit code.
func (c *testClient) runTestContainer(r io.Reader) (int, error) {
	log.Printf("Pulling image: %s", c.TestImage)
	pullOpts := docker.PullImageOptions{
		Repository: c.TestImage,
	}
	if err := c.DockerClient.PullImage(pullOpts, docker.AuthConfiguration{}); err != nil {
		return 0, fmt.Errorf("error pulling image: %s", err)
	}

	log.Printf("Creating container")
	containerOpts := docker.CreateContainerOptions{
		Config: &docker.Config{
			WorkingDir: filepath.Join(c.TestPath, c.RepoName),
			Image:      c.TestImage,
		},
	}
	container, err := c.DockerClient.CreateContainer(containerOpts)
	if err != nil {
		return 0, fmt.Errorf("error creating container: %s", err)
	}
	log.Printf("Copying source to container")
	err = c.DockerClient.UploadToContainer(container.ID, docker.UploadToContainerOptions{
		InputStream: r,
		Path:        c.TestPath,
	})
	if err != nil {
		return 0, fmt.Errorf("error adding source to container: %s", err)
	}
	log.Print("Starting container")
	if err := c.DockerClient.StartContainer(container.ID, nil); err != nil {
		return 0, fmt.Errorf("error starting container: %s", err)
	}

	// TODO: Stream this to the log or something so that people can see their
	// test output in real-time.
	var outBuf bytes.Buffer
	var errBuf bytes.Buffer
	attachChan := make(chan error, 1)
	go func() {
		err := c.DockerClient.AttachToContainer(docker.AttachToContainerOptions{
			Container:    container.ID,
			OutputStream: &outBuf,
			ErrorStream:  &errBuf,
			Logs:         true,
			Stdout:       true,
			Stderr:       true,
			Stream:       true,
		})
		attachChan <- err
	}()
	select {
	case err := <-attachChan:
		if err != nil {
			return 0, err
		}
	case <-time.After(c.Timeout):
		// Try to stop the container, but only fail if it isn't already stopped.
		err := c.DockerClient.StopContainer(container.ID, dockerKillTimeout)
		if _, ok := err.(*docker.ContainerNotRunning); !ok {
			return 0, err
		}
	}
	log.Printf("stdout:\n%s\n", outBuf.String())
	log.Printf("stderr:\n%s\n", errBuf.String())
	if err := ioutil.WriteFile(filepath.Join(c.Workspace, "log-stdout.txt"), outBuf.Bytes(), os.ModePerm); err != nil {
		return 0, err
	}
	if err := ioutil.WriteFile(filepath.Join(c.Workspace, "log-stderr.txt"), errBuf.Bytes(), os.ModePerm); err != nil {
		return 0, err
	}
	finalContainer, err := c.DockerClient.InspectContainer(container.ID)
	if err != nil {
		return 0, fmt.Errorf("error inspecting container: %s", err)
	}
	log.Print("Removing container")
	if err := c.DockerClient.RemoveContainer(docker.RemoveContainerOptions{ID: container.ID}); err != nil {
		return 0, fmt.Errorf("error removing container: %s", err)
	}
	return finalContainer.State.ExitCode, nil
}
