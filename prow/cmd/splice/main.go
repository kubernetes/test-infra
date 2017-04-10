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

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/plank"
)

var (
	submitQueueURL = flag.String("submit-queue-endpoint", "http://submit-queue.k8s.io/github-e2e-queue", "Submit Queue status URL")
	remoteURL      = flag.String("remote-url", "https://github.com/kubernetes/kubernetes", "Remote Git URL")
	orgName        = flag.String("org", "kubernetes", "Org name")
	repoName       = flag.String("repo", "kubernetes", "Repo name")
	logJson        = flag.Bool("log-json", false, "output log in JSON format")
	configPath     = flag.String("config-path", "/etc/config/config", "Where is config.yaml.")
	maxBatchSize   = flag.Int("batch-size", 5, "Maximum batch size")
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
			Number  int
			BaseRef string
		}
	}{}
	err = json.Unmarshal(body, &queue)
	if err != nil {
		return nil, err
	}

	ret := []int{}
	for _, e := range queue.E2EQueue {
		if e.BaseRef == "" || e.BaseRef == "master" {
			ret = append(ret, e.Number)
		}
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
	args := []string{"fetch", "-f", remote, "master:master"}
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

	out := []int{}
	for _, pr := range prs {
		err := s.gitCall("merge", "--no-ff", "--no-stat",
			"-m", fmt.Sprintf("merge #%d", pr),
			fmt.Sprintf("pr/%d", pr))
		if err != nil {
			// merge conflict: cleanup and move on
			err = s.gitCall("merge", "--abort")
			if err != nil {
				return nil, err
			}
			continue
		}
		out = append(out, pr)
	}
	return out, nil
}

// gitRef returns the SHA for the given git object-- a branch, generally.
func (s *splicer) gitRef(ref string) string {
	output, err := call("git", "-C", s.dir, "rev-parse", ref)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}

// Produce a kube.Refs for the given pull requests. This involves computing the
// git ref for master and the PRs.
func (s *splicer) makeBuildRefs(org, repo string, prs []int) kube.Refs {
	refs := kube.Refs{
		Org:     org,
		Repo:    repo,
		BaseRef: "master",
		BaseSHA: s.gitRef("master"),
	}
	for _, pr := range prs {
		branch := fmt.Sprintf("pr/%d", pr)
		refs.Pulls = append(refs.Pulls, kube.Pull{Number: pr, SHA: s.gitRef(branch)})
	}
	return refs
}

func main() {
	flag.Parse()

	if *logJson {
		log.SetFormatter(&log.JSONFormatter{})
	}
	log.SetLevel(log.DebugLevel)

	splicer, err := makeSplicer()
	if err != nil {
		log.WithError(err).Fatal("Could not make splicer.")
	}
	defer splicer.cleanup()

	ca := &config.ConfigAgent{}
	if err := ca.Start(*configPath); err != nil {
		log.WithError(err).Fatal("Could not start config agent.")
	}

	kc, err := kube.NewClientInCluster("default")
	if err != nil {
		log.WithError(err).Fatal("Error getting kube client.")
	}

	cooldown := 0
	// Loop endlessly, sleeping a minute between iterations
	for range time.Tick(1 * time.Minute) {
		// List batch jobs, only start a new one if none are active.
		currentJobs, err := kc.ListProwJobs(nil)
		if err != nil {
			log.WithError(err).Error("Error listing prow jobs.")
			continue
		}

		// Track successful batch runs -- we don't need to repeat them.
		succeeded := make(map[string]bool)

		running := []string{}
		for _, job := range currentJobs {
			if job.Complete() && job.Status.State == kube.SuccessState {
				ref := job.Spec.Refs.String()
				context := job.Spec.Context
				succeeded[ref+context] = true
			}
			if !job.Complete() {
				running = append(running, job.Spec.Job)
			}
		}
		if len(running) > 0 {
			log.Infof("Waiting on %d jobs: %v", len(running), running)
			continue
		}
		// Start a new batch if the cooldown is 0, otherwise wait. This gives
		// the SQ some time to merge before we start a new batch.
		if cooldown > 0 {
			cooldown--
			continue
		}
		queue, err := getQueuedPRs(*submitQueueURL)
		log.Info("PRs in queue:", queue)
		if err != nil {
			log.WithError(err).Warning("Error getting queued PRs. Is the submit queue down?")
			continue
		}
		batchPRs, err := splicer.findMergeable(*remoteURL, queue)
		if err != nil {
			log.WithError(err).Error("Error computing mergeable PRs.")
			continue
		}
		log.Infof("Batch PRs: %v", batchPRs)
		if len(batchPRs) <= 1 {
			continue
		}
		if len(batchPRs) > *maxBatchSize {
			batchPRs = batchPRs[:*maxBatchSize]
		}
		refs := splicer.makeBuildRefs(*orgName, *repoName, batchPRs)
		for _, job := range ca.Config().Presubmits[fmt.Sprintf("%s/%s", *orgName, *repoName)] {
			if job.AlwaysRun {
				if succeeded[refs.String()+job.Context] {
					log.Infof("not triggering job %v (already succeeded previously)", job.Name)
					continue
				}
				if _, err := kc.CreateProwJob(plank.NewProwJob(plank.BatchSpec(job, refs))); err != nil {
					log.WithError(err).WithField("job", job.Name).Error("Error starting job.")
				}
			}
		}
		cooldown = 5
	}
}
