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

package jobs

import (
	"fmt"
	"testing"
	"time"

	coreapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/kube"
)

func createTime(layout string, timeString string) metav1.Time {
	t, _ := time.Parse(layout, timeString)
	return metav1.NewTime(t)
}

type fkc []prowapi.ProwJob

func (f fkc) ListProwJobs(s string) ([]prowapi.ProwJob, error) {
	return f, nil
}

type fpkc string

func (f fpkc) GetLogs(name string, opts *coreapi.PodLogOptions) ([]byte, error) {
	if opts.Container != kube.TestContainerName {
		return nil, fmt.Errorf("wrong container: %s", opts.Container)
	}
	if name == "wowowow" || name == "powowow" {
		return []byte(f), nil
	}
	return nil, fmt.Errorf("pod not found: %s", name)
}

func TestGetLog(t *testing.T) {
	kc := fkc{
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "job",
			},
			Status: prowapi.ProwJobStatus{
				PodName: "wowowow",
				BuildID: "123",
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent:   prowapi.KubernetesAgent,
				Job:     "jib",
				Cluster: "trusted",
			},
			Status: prowapi.ProwJobStatus{
				PodName: "powowow",
				BuildID: "123",
			},
		},
	}
	ja := &JobAgent{
		kc:   kc,
		pkcs: map[string]PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")},
	}
	if err := ja.update(); err != nil {
		t.Fatalf("Updating: %v", err)
	}
	if res, err := ja.GetJobLog("job", "123"); err != nil {
		t.Fatalf("Failed to get log: %v", err)
	} else if got, expect := string(res), "clusterA"; got != expect {
		t.Errorf("Unexpected result getting logs for job 'job'. Expected %q, but got %q.", expect, got)
	}

	if res, err := ja.GetJobLog("jib", "123"); err != nil {
		t.Fatalf("Failed to get log: %v", err)
	} else if got, expect := string(res), "clusterB"; got != expect {
		t.Errorf("Unexpected result getting logs for job 'job'. Expected %q, but got %q.", expect, got)
	}
}

func TestProwJobs(t *testing.T) {
	kc := fkc{
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "jobFirst",
				Refs: &prowapi.Refs{
					Org:  "kubernetes",
					Repo: "test-infra",
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName:   "newpod",
				BuildID:   "1236",
				StartTime: createTime(time.RFC3339, "2008-01-02T15:04:05.999Z"),
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "jobThird",
				Refs: &prowapi.Refs{
					Org:  "kubernetes",
					Repo: "test-infra",
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName:   "wowowow",
				BuildID:   "1234",
				StartTime: createTime(time.RFC3339, "2006-01-02T15:04:05.999Z"),
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "jobSecond",
				Refs: &prowapi.Refs{
					Org:  "kubernetes",
					Repo: "test-infra",
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName:   "wowowow",
				BuildID:   "1235",
				StartTime: createTime(time.RFC3339, "2007-01-02T15:04:05.999Z"),
			},
		},
	}
	ja := &JobAgent{
		kc:   kc,
		pkcs: map[string]PodLogClient{kube.DefaultClusterAlias: fpkc("")},
	}
	if err := ja.update(); err != nil {
		t.Fatalf("Updating: %v", err)
	}

	pjs := ja.ProwJobs()
	if expect, got := 3, len(pjs); expect != got {
		t.Fatalf("Expected %d prowjobs, but got %d.", expect, got)
	}
	if expect, got := "kubernetes", pjs[0].Spec.Refs.Org; expect != got {
		t.Errorf("Expected prowjob to have org %q, but got %q.", expect, got)
	}
	if expect, got := "jobFirst", pjs[0].Spec.Job; expect != got {
		t.Errorf("Expected first prowjob to have job name %q, but got %q.", expect, got)
	}
	if expect, got := "jobSecond", pjs[1].Spec.Job; expect != got {
		t.Errorf("Expected second prowjob to have job name %q, but got %q.", expect, got)
	}
	if expect, got := "jobThird", pjs[2].Spec.Job; expect != got {
		t.Errorf("Expected third prowjob to have job name %q, but got %q.", expect, got)
	}
}

func TestJobs(t *testing.T) {
	kc := fkc{
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "jobFirst",
				Refs: &prowapi.Refs{
					Org:  "kubernetes",
					Repo: "test-infra",
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName:   "newpod",
				BuildID:   "1236",
				StartTime: createTime(time.RFC3339, "2008-01-02T15:04:05.999Z"),
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "jobThird",
				Refs: &prowapi.Refs{
					Org:  "kubernetes",
					Repo: "test-infra",
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName:   "wowowow",
				BuildID:   "1234",
				StartTime: createTime(time.RFC3339, "2006-01-02T15:04:05.999Z"),
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "jobSecond",
				Refs: &prowapi.Refs{
					Org:  "kubernetes",
					Repo: "test-infra",
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName:   "wowowow",
				BuildID:   "1235",
				StartTime: createTime(time.RFC3339, "2007-01-02T15:04:05.999Z"),
			},
		},
	}
	ja := &JobAgent{
		kc:   kc,
		pkcs: map[string]PodLogClient{kube.DefaultClusterAlias: fpkc("")},
	}
	if err := ja.update(); err != nil {
		t.Fatalf("Updating: %v", err)
	}

	jobs := ja.Jobs()
	if expect, got := 3, len(jobs); expect != got {
		t.Fatalf("Expected %d jobs, but got %d.", expect, got)
	}
	if expect, got := "kubernetes", jobs[0].Refs.Org; expect != got {
		t.Errorf("Expected jobs to have org %q, but got %q.", expect, got)
	}
	if expect, got := "jobFirst", jobs[0].Job; expect != got {
		t.Errorf("Expected first job to have job name %q, but got %q.", expect, got)
	}
	if expect, got := "jobSecond", jobs[1].Job; expect != got {
		t.Errorf("Expected second job to have job name %q, but got %q.", expect, got)
	}
	if expect, got := "jobThird", jobs[2].Job; expect != got {
		t.Errorf("Expected third job to have job name %q, but got %q.", expect, got)
	}
}
