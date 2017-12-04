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
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

type fkc []kube.ProwJob

func (f fkc) ListProwJobs(s string) ([]kube.ProwJob, error) {
	return f, nil
}

type fpkc struct{}

func (f fpkc) ListPods(selector string) ([]kube.Pod, error) {
	return nil, nil
}

func (f fpkc) GetLog(pod string) ([]byte, error) {
	if pod == "wowowow" {
		return []byte("wow"), nil
	}
	return nil, fmt.Errorf("pod not found: %s", pod)
}

func (f fpkc) GetLogStream(pod string, options map[string]string) (io.ReadCloser, error) {
	if pod == "wowowow" {
		return ioutil.NopCloser(bytes.NewBuffer([]byte("wow"))), nil
	}
	return nil, fmt.Errorf("pod not found: %s", pod)
}

type fcfg config.Config

func (f *fcfg) Config() *config.Config {
	return (*config.Config)(f)
}

func TestGetLog(t *testing.T) {
	kc := fkc{
		kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Agent: kube.KubernetesAgent,
				Job:   "job",
			},
			Status: kube.ProwJobStatus{
				PodName: "wowowow",
				BuildID: "123",
			},
		},
	}
	ja := &JobAgent{
		kc:  kc,
		pkc: &fpkc{},
		c:   &fcfg{},
	}
	if err := ja.update(); err != nil {
		t.Fatalf("Updating: %v", err)
	}
	if _, err := ja.GetJobLog("job", "123"); err != nil {
		t.Fatalf("Failed to get log: %v", err)
	}
}

func TestProwJobs(t *testing.T) {
	visibleOrg := "kubernetes"
	hiddenOrg := "kubernetes-shhh"
	kc := fkc{
		kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Agent: kube.KubernetesAgent,
				Job:   "job",
				Refs: kube.Refs{
					Org:  visibleOrg,
					Repo: "test-infra",
				},
			},
			Status: kube.ProwJobStatus{
				PodName: "wowowow",
				BuildID: "123",
			},
		},
		kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Agent: kube.KubernetesAgent,
				Job:   "jib",
				Refs: kube.Refs{
					Org:  hiddenOrg,
					Repo: "test-infra",
				},
			},
			Status: kube.ProwJobStatus{
				PodName: "wawawaw",
				BuildID: "456",
			},
		},
	}
	ja := &JobAgent{
		kc:  kc,
		pkc: &fpkc{},
		c: &fcfg{
			Deck: config.Deck{
				HiddenRepos: []string{hiddenOrg},
			},
		},
	}
	if err := ja.update(); err != nil {
		t.Fatalf("Updating: %v", err)
	}
	pjs := ja.ProwJobs()
	if expect, got := 1, len(pjs); expect != got {
		t.Fatalf("Expected %d prowjobs, but got %d.", expect, got)
	}
	if expect, got := visibleOrg, pjs[0].Spec.Refs.Org; expect != got {
		t.Errorf("Expected prowjob to have org %q, but got %q.", expect, got)
	}
}
