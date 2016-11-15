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
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
)

var (
	submitQueueURL = flag.String("submit-queue-endpoint", "http://submit-queue.k8s.io/github-e2e-queue", "Submit Queue status URL")
	remoteURL      = flag.String("remote-url", "https://github.com/kubernetes/kubernetes", "Remote Git URL")
	repoName       = flag.String("repo", "kubernetes/kubernetes", "Repo name")
	logJson        = flag.Bool("logjson", false, "output log in JSON format")
)

// Call a binary and return its output and success status.
func call(binary string, args ...string) (string, error) {
	cmdout := "+ " + binary + " "
	for _, arg := range args {
		cmdout += arg + " "
	}
	log.Debug(cmdout)

	cmd := exec.Command(binary, args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// getQueuedPRs reads the list of queued PRs from the Submit Queue.
func getQueuedPRs(url string) ([]int, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	queue := struct {
		E2EQueue []struct {
			Number int
		}
	}{}
	err = json.Unmarshal(body, &queue)
	if err != nil {
		return nil, err
	}

	ret := []int{}
	for _, e := range queue.E2EQueue {
		ret = append(ret, e.Number)
	}
	return ret, nil
}

// Splicer manages a git repo in specific directory.
type splicer struct {
	dir string // The repository location.
}

// makeSplicer returns a splicer in a new temporary directory,
// with an initial .git dir created.
func makeSplicer() (*splicer, error) {
	dir, err := ioutil.TempDir("", "splice_")
	if err != nil {
		return nil, err
	}
	s := &splicer{dir}
	err = s.gitCalls([][]string{
		{"init"},
		{"config", "--local", "user.name", "K8S Prow Splice"},
		{"config", "--local", "user.email", "splice@localhost"},
	})
	if err != nil {
		s.cleanup()
		return nil, err
	}
	log.Debug("splicer created in", dir)
	return s, nil
}

// cleanup recurisvely deletes the repository
func (s *splicer) cleanup() {
	os.RemoveAll(s.dir)
}

// gitCall is a helper to call `git -C $path $args`.
func (s *splicer) gitCall(args ...string) error {
	fullArgs := append([]string{"-C", s.dir}, args...)
	output, err := call("git", fullArgs...)
	if len(output) > 0 {
		log.Debug(output)
	}
	return err
}

// gitCalls is a helper to chain repeated gitCall invocations,
// returning the first failure, or nil if they all succeeded.
func (s *splicer) gitCalls(argsList [][]string) error {
	for _, args := range argsList {
		err := s.gitCall(args...)
		if err != nil {
			return err
		}
	}
	return nil
}

// findMergeable fetches given PRs from upstream, merges them locally,
// and finally returns a list of PRs that can be merged without conflicts.
func (s *splicer) findMergeable(remote string, prs []int) ([]int, error) {
	args := []string{"fetch", remote, "master:master"}
	for _, pr := range prs {
		args = append(args, fmt.Sprintf("pull/%d/head:pr/%d", pr, pr))
	}

	err := s.gitCalls([][]string{
		{"reset", "--hard"},
		{"checkout", "--orphan", "blank"},
		{"reset", "--hard"},
		{"clean", "-fdx"},
		args,
		{"checkout", "-B", "batch", "master"},
	})
	if err != nil {
		return nil, err
	}

	for i, pr := range prs {
		err := s.gitCall("merge", "--no-ff", "--no-stat",
			"-m", fmt.Sprintf("merge #%d", pr),
			fmt.Sprintf("pr/%d", pr))
		if err != nil {
			return prs[:i], nil
		}
	}
	return prs, nil
}

// gitRef returns the SHA for the given git object-- a branch, generally.
func (s *splicer) gitRef(ref string) string {
	output, err := call("git", "-C", s.dir, "rev-parse", ref)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}

type prRef struct {
	PR  int    `json:"pr"`
	SHA string `json:"sha"`
}

type buildRef struct {
	Repo string  `json:"repo"`
	Base string  `json:"base_ref"`
	PRs  []prRef `json:"prs"`
}

// Produce a struct describing the exact revisions that were merged together.
// This is sent to Prow and used by the Submit Queue when deciding whether it
// can use a batch test result to merge something.
func (s *splicer) makeBuildRef(repo string, prs []int) *buildRef {
	ref := &buildRef{Repo: repo, Base: s.gitRef("master")}
	for _, pr := range prs {
		branch := fmt.Sprintf("pr/%d", pr)
		ref.PRs = append(ref.PRs, prRef{pr, s.gitRef(branch)})
	}
	return ref
}

func main() {
	flag.Parse()

	if *logJson {
		log.SetFormatter(&log.JSONFormatter{})
	}
	log.SetLevel(log.DebugLevel)

	splicer, err := makeSplicer()
	if err != nil {
		log.Fatal(err)
	}
	defer splicer.cleanup()

	// Loop endless, sleeping a minute between iterations
	for {
		queue, err := getQueuedPRs(*submitQueueURL)
		log.Info("PRs in queue:", queue)
		if err != nil {
			log.WithError(err).Error("error getting queued PRs")
			time.Sleep(1 * time.Minute)
			continue
		}
		batchPRs, err := splicer.findMergeable(*remoteURL, queue)
		if err != nil {
			log.WithError(err).Error("error computing mergeable PRs")
			time.Sleep(1 * time.Minute)
			continue
		}
		buildRef := splicer.makeBuildRef(*repoName, batchPRs)
		buf, err := json.Marshal(buildRef)
		if err != nil {
			log.WithError(err).Fatal("unable to marshal JSON")
		}
		log.Info("batch PRs:", batchPRs)
		log.Info("batch buildRef:", string(buf))
		time.Sleep(1 * time.Minute)
	}
}
