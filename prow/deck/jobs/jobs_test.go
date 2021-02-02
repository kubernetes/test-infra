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
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
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

func (f fpkc) GetLogs(name, container string) ([]byte, error) {
	if name == "wowowow" || name == "powowow" {
		return []byte(fmt.Sprintf("%s.%s", f, container)), nil
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
	if res, err := ja.GetJobLog("job", "123", kube.TestContainerName); err != nil {
		t.Fatalf("Failed to get log: %v", err)
	} else if got, expect := string(res), fmt.Sprintf("clusterA.%s", kube.TestContainerName); got != expect {
		t.Errorf("Unexpected result getting logs for job 'job'. Expected %q, but got %q.", expect, got)
	}

	if res, err := ja.GetJobLog("jib", "123", kube.TestContainerName); err != nil {
		t.Fatalf("Failed to get log: %v", err)
	} else if got, expect := string(res), fmt.Sprintf("clusterB.%s", kube.TestContainerName); got != expect {
		t.Errorf("Unexpected result getting logs for job 'job'. Expected %q, but got %q.", expect, got)
	}

	customContainerName := "custom-container-name"
	if res, err := ja.GetJobLog("jib", "123", customContainerName); err != nil {
		t.Fatalf("Failed to get log: %v", err)
	} else if got, expect := string(res), fmt.Sprintf("clusterB.%s", customContainerName); got != expect {
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

func TestListProwJobs(t *testing.T) {
	templateJob := &prowapi.ProwJob{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "prowjobs",
		},
	}

	var testCases = []struct {
		name        string
		selector    string
		prowJobs    []func(*prowapi.ProwJob) runtime.Object
		listErr     bool
		hiddenRepos sets.String
		hiddenOnly  bool
		showHidden  bool
		expected    sets.String
		expectedErr bool
	}{
		{
			name:        "list error results in filter error",
			listErr:     true,
			expectedErr: true,
		},
		{
			name:     "no hidden repos returns all prowjobs",
			selector: labels.Everything().String(),
			prowJobs: []func(*prowapi.ProwJob) runtime.Object{
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "first"
					return in
				},
			},
			expected: sets.NewString("first"),
		},
		{
			name:     "no hidden repos returns all prowjobs except those not matching label selector",
			selector: "foo=bar",
			prowJobs: []func(*prowapi.ProwJob) runtime.Object{
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "first"
					return in
				},
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "second"
					in.Labels = map[string]string{"foo": "bar"}
					return in
				},
			},
			expected: sets.NewString("second"),
		},
		{
			name:     "hidden repos excludes prowjobs from those repos",
			selector: labels.Everything().String(),
			prowJobs: []func(*prowapi.ProwJob) runtime.Object{
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "first"
					return in
				},
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "second"
					in.Spec.Refs = &prowapi.Refs{
						Org:  "org",
						Repo: "repo",
					}
					return in
				},
			},
			hiddenRepos: sets.NewString("org/repo"),
			expected:    sets.NewString("first"),
		},
		{
			name:     "hidden repos doesn't exclude prowjobs from other repos",
			selector: labels.Everything().String(),
			prowJobs: []func(*prowapi.ProwJob) runtime.Object{
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "first"
					return in
				},
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "second"
					in.Spec.Refs = &prowapi.Refs{
						Org:  "org",
						Repo: "other",
					}
					return in
				},
			},
			hiddenRepos: sets.NewString("org/repo"),
			expected:    sets.NewString("first", "second"),
		},
		{
			name:     "hidden orgs excludes prowjobs from those orgs",
			selector: labels.Everything().String(),
			prowJobs: []func(*prowapi.ProwJob) runtime.Object{
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "first"
					return in
				},
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "second"
					in.Spec.Refs = &prowapi.Refs{
						Org:  "org",
						Repo: "other",
					}
					return in
				},
			},
			hiddenRepos: sets.NewString("org"),
			expected:    sets.NewString("first"),
		},
		{
			name:     "hidden orgs doesn't exclude prowjobs from other orgs",
			selector: labels.Everything().String(),
			prowJobs: []func(*prowapi.ProwJob) runtime.Object{
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "first"
					return in
				},
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "second"
					in.Spec.Refs = &prowapi.Refs{
						Org:  "other",
						Repo: "other",
					}
					return in
				},
			},
			hiddenRepos: sets.NewString("org"),
			expected:    sets.NewString("first", "second"),
		},
		{
			name:     "hidden repos excludes prowjobs from those repos even by extra_refs",
			selector: labels.Everything().String(),
			prowJobs: []func(*prowapi.ProwJob) runtime.Object{
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "first"
					in.Spec.ExtraRefs = []prowapi.Refs{{Org: "org", Repo: "repo"}}
					return in
				},
			},
			hiddenRepos: sets.NewString("org/repo"),
			expected:    sets.NewString(),
		},
		{
			name:     "hidden orgs excludes prowjobs from those orgs even by extra_refs",
			selector: labels.Everything().String(),
			prowJobs: []func(*prowapi.ProwJob) runtime.Object{
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "first"
					in.Spec.ExtraRefs = []prowapi.Refs{{Org: "org", Repo: "repo"}}
					return in
				},
			},
			hiddenRepos: sets.NewString("org"),
			expected:    sets.NewString(),
		},
		{
			name:     "prowjobs without refs are returned even with hidden repos filtering",
			selector: labels.Everything().String(),
			prowJobs: []func(*prowapi.ProwJob) runtime.Object{
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "first"
					return in
				},
			},
			hiddenRepos: sets.NewString("org/repo"),
			expected:    sets.NewString("first"),
		},
		{
			name:     "all prowjobs are returned when showHidden is true",
			selector: labels.Everything().String(),
			prowJobs: []func(*prowapi.ProwJob) runtime.Object{
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "first"
					in.Spec.ExtraRefs = []prowapi.Refs{{Org: "org", Repo: "repo"}}
					return in
				},
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "second"
					return in
				},
			},
			hiddenRepos: sets.NewString("org/repo"),
			expected:    sets.NewString("first", "second"),
			showHidden:  true,
		},
		{
			name: "setting pj.Spec.Hidden hides it",
			prowJobs: []func(*prowapi.ProwJob) runtime.Object{
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "hidden"
					in.Spec.Hidden = true
					return in
				},
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "shown"
					return in
				},
			},
			expected: sets.NewString("shown"),
		},
		{
			name: "hidden repo or org in extra_refs hides it",
			prowJobs: []func(*prowapi.ProwJob) runtime.Object{
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "hidden-repo"
					in.Spec.ExtraRefs = []prowapi.Refs{{Org: "hide", Repo: "me"}}
					return in
				},
				func(in *prowapi.ProwJob) runtime.Object {
					in.Name = "hidden-org"
					in.Spec.ExtraRefs = []prowapi.Refs{{Org: "hidden-org"}}
					return in
				},
			},
			hiddenRepos: sets.NewString("hide/me", "hidden-org"),
		},
	}

	for _, testCase := range testCases {
		var data []runtime.Object
		for _, generator := range testCase.prowJobs {
			data = append(data, generator(templateJob.DeepCopy()))
		}
		fakeProwJobClient := &possiblyErroringFakeCtrlRuntimeClient{
			Client:      fakectrlruntimeclient.NewFakeClient(data...),
			shouldError: testCase.listErr,
		}
		lister := filteringProwJobLister{
			client: fakeProwJobClient,
			hiddenRepos: func() sets.String {
				return testCase.hiddenRepos
			},
			hiddenOnly: testCase.hiddenOnly,
			showHidden: testCase.showHidden,
			cfg:        func() *config.Config { return &config.Config{} },
		}

		filtered, err := lister.ListProwJobs(testCase.selector)
		if err == nil && testCase.expectedErr {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}
		if err != nil && !testCase.expectedErr {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, err)
		}

		filteredNames := sets.NewString()
		for _, prowJob := range filtered {
			filteredNames.Insert(prowJob.Name)
		}

		if missing := testCase.expected.Difference(filteredNames); missing.Len() > 0 {
			t.Errorf("%s: did not get expected jobs in filtered list: %v", testCase.name, missing.List())
		}
		if extra := filteredNames.Difference(testCase.expected); extra.Len() > 0 {
			t.Errorf("%s: got unexpected jobs in filtered list: %v", testCase.name, extra.List())
		}
	}
}

type possiblyErroringFakeCtrlRuntimeClient struct {
	ctrlruntimeclient.Client
	shouldError bool
}

func (p *possiblyErroringFakeCtrlRuntimeClient) List(
	ctx context.Context,
	pjl *prowapi.ProwJobList,
	opts ...ctrlruntimeclient.ListOption) error {
	if p.shouldError {
		return errors.New("could not list ProwJobs")
	}
	return p.Client.List(ctx, pjl, opts...)
}
