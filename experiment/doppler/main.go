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

// Doppler provides flake detection and registration of all flaky job
// statistics as prometheus metrics. Rewritten from Python (../flakedetection.py)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type prowJob struct {
	Type    string `json:"type"`
	Repo    string `json:"repo"`
	PullSHA string `json:"pull_sha"`
	Job     string `json:"job"`
	State   string `json:"state"`
}

type jobResults struct {
	flakeCount  int
	commitCount int
	chance      float64
	jobName     string
}

func (f jobResults) String() string {
	return fmt.Sprintf("%d/%d\t(%.2f%%)\t%s", f.flakeCount, f.commitCount, f.chance, f.jobName)
}

type byChance []jobResults

func (ft byChance) Len() int           { return len(ft) }
func (ft byChance) Swap(i, j int)      { ft[i], ft[j] = ft[j], ft[i] }
func (ft byChance) Less(i, j int) bool { return ft[i].chance > ft[j].chance }

type options struct {
	prowURL string
	path    string
	runOnce bool
	repo    string
}

func flagOptions() options {
	o := options{}
	flag.StringVar(&o.prowURL, "prow-url", "https://prow.k8s.io", "Prow frontend base URL. Required for reading job results")
	flag.BoolVar(&o.runOnce, "run-once", false, "Run once and exit")
	flag.StringVar(&o.repo, "repo", "kubernetes/kubernetes", "Gather results for a specific repository.")
	flag.Parse()
	return o
}

func readHTTP(url string) ([]byte, error) {
	var err error
	retryDelay := time.Duration(2) * time.Second
	for retryCount := 0; retryCount < 5; retryCount++ {
		if retryCount > 0 {
			time.Sleep(retryDelay)
			retryDelay *= time.Duration(2)
		}
		resp, err := http.Get(url)
		if resp != nil && resp.StatusCode >= 500 {
			continue
		}
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			continue
		}
		return body, nil
	}
	return nil, fmt.Errorf("ran out of retries reading from %q. The last error was: %v", url, err)
}

func listProwJobs(url string) ([]prowJob, error) {
	body, err := readHTTP(url + "/data.js")
	if err != nil {
		return nil, fmt.Errorf("error reading jobs from prow: %v", err)
	}

	var jobs []prowJob
	if err := json.Unmarshal(body, &jobs); err != nil {
		return nil, fmt.Errorf("error unmarshaling prowjobs: %v", err)
	}
	return jobs, nil
}

func syncOne(o options, metrics Metrics) error {
	jobs, err := listProwJobs(o.prowURL)
	if err != nil {
		return err
	}

	// Partition the jobs per repository.
	validJobs := make(map[string][]prowJob)
	for _, job := range jobs {
		if job.Type != "presubmit" {
			continue
		}
		if o.repo != "" && job.Repo != o.repo {
			continue
		}
		if job.State != "success" && job.State != "failure" {
			continue
		}
		validJobs[job.Repo] = append(validJobs[job.Repo], job)
	}

	for repo, jobs := range validJobs {
		jobMap := make(map[string]map[string][]string)
		commitMap := make(map[string]map[string][]string)
		for _, job := range jobs {
			// populate jobMap
			if _, ok := jobMap[job.Job]; !ok {
				jobMap[job.Job] = make(map[string][]string)
			}
			if _, ok := jobMap[job.Job][job.PullSHA]; !ok {
				jobMap[job.Job][job.PullSHA] = make([]string, 0)
			}
			jobMap[job.Job][job.PullSHA] = append(jobMap[job.Job][job.PullSHA], job.State)
			// populate commitMap
			if _, ok := commitMap[job.PullSHA]; !ok {
				commitMap[job.PullSHA] = make(map[string][]string)
			}
			if _, ok := commitMap[job.PullSHA][job.Job]; !ok {
				commitMap[job.PullSHA][job.Job] = make([]string, 0)
			}
			commitMap[job.PullSHA][job.Job] = append(commitMap[job.PullSHA][job.Job], job.State)
		}
		jobCommits := make(map[string]int)
		jobFlakes := make(map[string]int)
		for job, commits := range jobMap {
			jobCommits[job] = len(commits)
			jobFlakes[job] = 0
			for _, results := range commits {
				hasFailure, hasSuccess := false, false
				for _, state := range results {
					if state == "success" {
						hasSuccess = true
					}
					if state == "failure" {
						hasFailure = true
					}
					if hasSuccess && hasFailure {
						break
					}
				}
				if hasSuccess && hasFailure {
					jobFlakes[job]++
				}
			}
		}

		var flaky []jobResults
		for job, flakeCount := range jobFlakes {
			if jobCommits[job] < 10 {
				continue
			}
			failChance := float64(flakeCount) / float64(jobCommits[job])
			flaky = append(flaky, jobResults{
				flakeCount:  flakeCount,
				commitCount: jobCommits[job],
				chance:      100 * failChance,
				jobName:     job,
			})
		}

		fmt.Printf("Certain flakes in %s:\n", repo)
		sort.Sort(byChance(flaky))
		for _, flake := range flaky {
			fmt.Println(flake)
			metrics.Set(repo, flake.jobName, flake.chance)
		}

		flaked := 0
		for _, jobs := range commitMap {
		out:
			for _, results := range jobs {
				hasFailure, hasSuccess := false, false
				for _, state := range results {
					if state == "success" {
						hasSuccess = true
					}
					if state == "failure" {
						hasFailure = true
					}
					if hasSuccess && hasFailure {
						flaked++
						break out
					}
				}
			}
		}
		flakeChance := float64((flaked * 100.0)) / float64(len(commitMap))
		metrics.Set(repo, total, flakeChance)
		fmt.Printf("Commits that flaked in %s: %d/%d %.2f%%\n", repo, flaked, len(commitMap), flakeChance)
	}
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	o := flagOptions()

	// TODO: Refresh metrics
	metrics, err := RegisterMetrics(o.prowURL, o.repo)
	if err != nil {
		log.Fatalf("could not load metrics: %v", err)
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		for {
			err := syncOne(o, metrics)
			if err != nil {
				fmt.Printf("error syncing: %v", err)
				return
			}
			if o.runOnce {
				return
			}
			time.Sleep(time.Minute)
		}
	}()

	if o.runOnce {
		wg.Wait()
		return
	}

	http.Handle("/prometheus", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":8080", nil))
}
