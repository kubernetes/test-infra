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

package mungers

import (
	"encoding/json"

	"k8s.io/test-infra/mungegithub/mungers/mungerutil"
)

// xref k8s.io/test-infra/prow/cmd/deck/jobs.go
type prowJob struct {
	Type     string `json:"type"`
	Repo     string `json:"repo"`
	Refs     string `json:"refs"`
	Number   int    `json:"number"`
	BuildID  string `json:"build_id"`
	Job      string `json:"job"`
	Finished string `json:"finished"`
	State    string `json:"state"`
	Context  string `json:"context"`
	URL      string `json:"url"`
}

type prowJobs []prowJob

// getJobs reads job information as JSON from a given URL.
func getJobs(url string) (prowJobs, error) {
	body, err := mungerutil.ReadHTTP(url)
	if err != nil {
		return nil, err
	}

	jobs := prowJobs{}
	err = json.Unmarshal(body, &jobs)
	if err != nil {
		return nil, err
	}
	return jobs, nil
}

func (j prowJobs) filter(pred func(prowJob) bool) prowJobs {
	out := prowJobs{}
	for _, job := range j {
		if pred(job) {
			out = append(out, job)
		}
	}
	return out
}

func (j prowJobs) repo(repo string) prowJobs {
	return j.filter(func(job prowJob) bool { return job.Repo == repo })
}

func (j prowJobs) batch() prowJobs {
	return j.filter(func(job prowJob) bool { return job.Type == "batch" })
}

func (j prowJobs) successful() prowJobs {
	return j.filter(func(job prowJob) bool { return job.State == "success" })
}

func (j prowJobs) firstUnfinished() *prowJob {
	for _, job := range j {
		if job.State == "triggered" || job.State == "pending" {
			return &job
		}
	}
	return nil
}
