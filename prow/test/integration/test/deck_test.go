/*
Copyright 2021 The Kubernetes Authors.

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

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

var (
	DefaultID = prowjobv1.ProwJob{
		ObjectMeta: v1.ObjectMeta{
			Name:      "defaultid",
			Namespace: defaultNamespace,
			Labels: map[string]string{
				"created-by-prow": "true",
			},
		},
		Spec: prowjobv1.ProwJobSpec{
			Job: "Default TenantID",
			ProwJobDefault: &prowjobv1.ProwJobDefault{
				TenantID: config.DefaultTenantID,
			},
			Namespace: testpodNamespace,
		},
		Status: prowjobv1.ProwJobStatus{
			State:     prowjobv1.TriggeredState,
			StartTime: v1.NewTime(time.Now()),
		},
	}
	NoID = prowjobv1.ProwJob{
		ObjectMeta: v1.ObjectMeta{
			Name:      "noid",
			Namespace: defaultNamespace,
			Labels: map[string]string{
				"created-by-prow": "true",
			},
		},
		Spec: prowjobv1.ProwJobSpec{
			Job:            "No TenantID",
			ProwJobDefault: &prowjobv1.ProwJobDefault{},
			Namespace:      testpodNamespace,
		},
		Status: prowjobv1.ProwJobStatus{
			State:     prowjobv1.TriggeredState,
			StartTime: v1.NewTime(time.Now()),
		},
	}
	NoDefault = prowjobv1.ProwJob{
		ObjectMeta: v1.ObjectMeta{
			Name:      "nodefault",
			Namespace: defaultNamespace,
			Labels: map[string]string{
				"created-by-prow": "true",
			},
		},
		Spec: prowjobv1.ProwJobSpec{
			Job:       "No ProwJobDefault",
			Namespace: testpodNamespace,
		},
		Status: prowjobv1.ProwJobStatus{
			State:     prowjobv1.TriggeredState,
			StartTime: v1.NewTime(time.Now()),
		},
	}
	DefaultIDHidden = prowjobv1.ProwJob{
		ObjectMeta: v1.ObjectMeta{
			Name:      "defaulthidden",
			Namespace: defaultNamespace,
			Labels: map[string]string{
				"created-by-prow": "true",
			},
		},
		Spec: prowjobv1.ProwJobSpec{
			Job: "Default TenantID and Hidden",
			ProwJobDefault: &prowjobv1.ProwJobDefault{
				TenantID: config.DefaultTenantID,
			},
			Hidden:    true,
			Namespace: testpodNamespace,
		},
		Status: prowjobv1.ProwJobStatus{
			State:     prowjobv1.TriggeredState,
			StartTime: v1.NewTime(time.Now()),
		},
	}
	NoIDHidden = prowjobv1.ProwJob{
		ObjectMeta: v1.ObjectMeta{
			Name:      "nohiddenid",
			Namespace: defaultNamespace,
			Labels: map[string]string{
				"created-by-prow": "true",
			},
		},
		Spec: prowjobv1.ProwJobSpec{
			Job:            "No TenantID and Hidden",
			ProwJobDefault: &prowjobv1.ProwJobDefault{},
			Hidden:         true,
			Namespace:      testpodNamespace,
		},
		Status: prowjobv1.ProwJobStatus{
			State:     prowjobv1.TriggeredState,
			StartTime: v1.NewTime(time.Now()),
		},
	}
	NoDefaultHidden = prowjobv1.ProwJob{
		ObjectMeta: v1.ObjectMeta{
			Name:      "nodefaulthidden",
			Namespace: defaultNamespace,
			Labels: map[string]string{
				"created-by-prow": "true",
			},
		},
		Spec: prowjobv1.ProwJobSpec{
			Job:       "No ProwJobDefault and Hidden",
			Hidden:    true,
			Namespace: testpodNamespace,
		},
		Status: prowjobv1.ProwJobStatus{
			State:     prowjobv1.TriggeredState,
			StartTime: v1.NewTime(time.Now()),
		},
	}
	ID = prowjobv1.ProwJob{
		ObjectMeta: v1.ObjectMeta{
			Name:      "id",
			Namespace: defaultNamespace,
			Labels: map[string]string{
				"created-by-prow": "true",
			},
		},
		Spec: prowjobv1.ProwJobSpec{
			Job: "TenantID and hidden",
			ProwJobDefault: &prowjobv1.ProwJobDefault{
				TenantID: "tester",
			},
			Namespace: testpodNamespace,
		},
		Status: prowjobv1.ProwJobStatus{
			State:     prowjobv1.TriggeredState,
			StartTime: v1.NewTime(time.Now()),
		},
	}
	IDHidden = prowjobv1.ProwJob{
		ObjectMeta: v1.ObjectMeta{
			Name:      "idhidden",
			Namespace: defaultNamespace,
			Labels: map[string]string{
				"created-by-prow": "true",
			},
		},
		Spec: prowjobv1.ProwJobSpec{
			Job: "Default TenantID and Hidden",
			ProwJobDefault: &prowjobv1.ProwJobDefault{
				TenantID: "tester",
			},
			Hidden:    true,
			Namespace: testpodNamespace,
		},
		Status: prowjobv1.ProwJobStatus{
			State:     prowjobv1.TriggeredState,
			StartTime: v1.NewTime(time.Now()),
		},
	}
)

func populateProwJobs(t *testing.T, prowjobs *prowjobv1.ProwJobList, kubeClient ctrlruntimeclient.Client, ctx context.Context) {
	if len(prowjobs.Items) > 0 {
		for _, prowjob := range prowjobs.Items {
			t.Logf("Creating prowjob: %s", prowjob.Name)

			if err := kubeClient.Create(ctx, &prowjob); err != nil {
				t.Fatalf("Failed creating prowjob: %v", err)
			}
			t.Logf("Finished creating prowjob: %s", prowjob.Name)
		}
	}
}

func getCleanupProwJobsFunc(prowjobs *prowjobv1.ProwJobList, kubeClient ctrlruntimeclient.Client, ctx context.Context) func() {
	return func() {
		for _, prowjob := range prowjobs.Items {
			kubeClient.Delete(ctx, &prowjob)
		}
	}
}

func getSpecs(pjs *prowjobv1.ProwJobList) []prowjobv1.ProwJobSpec {
	res := []prowjobv1.ProwJobSpec{}
	for _, pj := range pjs.Items {
		res = append(res, pj.Spec)
	}
	return res
}

func TestDeck(t *testing.T) {
	t.Parallel()

	resp, err := http.Get("http://localhost/deck")
	if err != nil {
		t.Fatalf("Failed getting deck front end %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Expected response status code %d, got %d, ", http.StatusOK, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed getting deck body response content %v", err)
	}
	if got, want := string(body), "<title>Prow Status</title>"; !strings.Contains(got, want) {
		firstLines := strings.Join(strings.SplitN(strings.TrimSpace(got), "\n", 30), "\n")
		t.Fatalf("Expected content %q not found in body %s [......]", want, firstLines)
	}
}

func TestDeckTenantIDs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		prowjobs     *prowjobv1.ProwJobList
		expected     *prowjobv1.ProwJobList
		unexpected   *prowjobv1.ProwJobList
		deckInstance string
	}{
		{
			name:         "deck-tenanted",
			prowjobs:     &prowjobv1.ProwJobList{Items: []prowjobv1.ProwJob{DefaultID, NoID, NoDefault, DefaultIDHidden, NoIDHidden, NoDefaultHidden, ID, IDHidden}},
			expected:     &prowjobv1.ProwJobList{Items: []prowjobv1.ProwJob{ID, IDHidden}},
			unexpected:   &prowjobv1.ProwJobList{Items: []prowjobv1.ProwJob{DefaultID, NoID, NoDefault, DefaultIDHidden, NoIDHidden, NoDefaultHidden}},
			deckInstance: "deck-tenanted",
		},
		{
			name:         "public-deck",
			prowjobs:     &prowjobv1.ProwJobList{Items: []prowjobv1.ProwJob{DefaultID, NoID, NoDefault, DefaultIDHidden, NoIDHidden, NoDefaultHidden, ID, IDHidden}},
			expected:     &prowjobv1.ProwJobList{Items: []prowjobv1.ProwJob{DefaultID, NoID, NoDefault}},
			unexpected:   &prowjobv1.ProwJobList{Items: []prowjobv1.ProwJob{ID, IDHidden, DefaultIDHidden, NoIDHidden, NoDefaultHidden}},
			deckInstance: "deck",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			//Give them new names to prevent conflict
			name := RandomString(t)
			prowjobs := renamePJs(tt.prowjobs, name)
			expected := renamePJs(tt.expected, name)
			unexpected := renamePJs(tt.unexpected, name)

			clusterContext := getClusterContext()
			t.Logf("Creating client for cluster: %s", clusterContext)
			kubeClient, err := NewClients("", clusterContext)
			if err != nil {
				t.Fatalf("Failed creating clients for cluster %q: %v", clusterContext, err)
			}
			ctx := context.Background()

			populateProwJobs(t, &prowjobs, kubeClient, ctx)
			t.Cleanup(getCleanupProwJobsFunc(&prowjobs, kubeClient, ctx))

			// Give it some time
			time.Sleep(30 * time.Second)
			resp, err := http.Get(fmt.Sprintf("http://localhost/%s/prowjobs.js", tt.deckInstance))
			if err != nil {
				t.Fatalf("Failed getting deck-tenanted front end %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("Expected response status code %d, got %d, ", http.StatusOK, resp.StatusCode)
			}
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("Failed getting deck body response content %v", err)
			}

			got := prowjobv1.ProwJobList{}
			if err = yaml.Unmarshal(body, &got); err != nil {
				t.Fatalf("Failed unmarshal prowjobs %v", err)
			}

			if allExpected := expectedPJsInDeck(&expected, &got); !allExpected {
				t.Fatalf("Not all expected PJs are present. want: %v\n got:%v", expected, got)
			}

			if unexpectedFound := unexpectedPJsInDeck(&unexpected, &got); unexpectedFound {
				t.Fatalf("Unexpected PJ is present. want: %v\n got:%v", expected, got)
			}
		})
	}
}

func TestRerun(t *testing.T) {
	t.Parallel()
	const rerunJobConfigFile = "rerun-test.yaml"
	jobName := "rerun-test-job-" + RandomString(t)
	var rerunJobConfigTemplate = `periodics:
- interval: 1h
  name: %s
  spec:
    containers:
    - command:
      - echo
      args:
      - "Hello World!"
      image: localhost:5001/alpine
  labels:
    foo: "%s"`

	clusterContext := getClusterContext()
	t.Logf("Creating client for cluster: %s", clusterContext)
	kubeClient, err := NewClients("", clusterContext)
	if err != nil {
		t.Fatalf("Failed creating clients for cluster %q: %v", clusterContext, err)
	}

	rerunJobConfig := fmt.Sprintf(rerunJobConfigTemplate, jobName, "foo")
	if err := updateJobConfig(context.Background(), kubeClient, rerunJobConfigFile, []byte(rerunJobConfig)); err != nil {
		t.Fatalf("Failed update job config: %v", err)
	}

	// Now we are waiting on Horologium to create the first prow job so that we
	// can rerun from.
	// Horologium itself is pretty good at handling the configmap update, but
	// not kubelet, accoriding to
	// https://github.com/kubernetes/kubernetes/issues/30189 kubelet syncs
	// configmap updates on existing pods every minute, which is a long wait.
	// The proposed fix in the issue was updating the deployment, which imo
	// should be better handled by just refreshing pods.
	// So here comes forcing restart of horologium pods.
	if err := refreshProwPods(kubeClient, context.Background(), "horologium"); err != nil {
		t.Fatalf("Failed refreshing horologium pods: %v", err)
	}
	// Same with deck
	if err := refreshProwPods(kubeClient, context.Background(), "deck"); err != nil {
		t.Fatalf("Failed refreshing deck pods: %v", err)
	}

	t.Cleanup(func() {
		if err := updateJobConfig(context.Background(), kubeClient, rerunJobConfigFile, []byte{}); err != nil {
			t.Logf("ERROR CLEANUP: %v", err)
		}
		labels, err := labels.Parse("prow.k8s.io/job = " + jobName)
		if err != nil {
			t.Logf("Skip cleaning up jobs, as failed parsing label: %v", err)
			return
		}
		if err := kubeClient.DeleteAllOf(context.Background(), &prowjobv1.ProwJob{}, &ctrlruntimeclient.DeleteAllOfOptions{
			ListOptions: ctrlruntimeclient.ListOptions{LabelSelector: labels},
		}); err != nil {
			t.Logf("ERROR CLEANUP: %v", err)
		}
	})
	ctx := context.Background()
	getLatestJob := func(t *testing.T, jobName string, lastRun *v1.Time) *prowjobv1.ProwJob {
		var res *prowjobv1.ProwJob
		if err := wait.Poll(time.Second, 90*time.Second, func() (bool, error) {
			pjs := &prowjobv1.ProwJobList{}
			err = kubeClient.List(ctx, pjs, &ctrlruntimeclient.ListOptions{
				LabelSelector: labels.SelectorFromSet(map[string]string{kube.ProwJobAnnotation: jobName}),
				Namespace:     defaultNamespace,
			})
			if err != nil {
				return false, fmt.Errorf("failed listing prow jobs: %w", err)
			}
			sort.Slice(pjs.Items, func(i, j int) bool {
				return pjs.Items[i].Status.StartTime.After(pjs.Items[j].Status.StartTime.Time)
			})
			if len(pjs.Items) > 0 {
				if lastRun != nil && pjs.Items[0].CreationTimestamp.Before(lastRun) {
					return false, nil
				}
				res = &pjs.Items[0]
			}
			return res != nil, nil
		}); err != nil {
			t.Fatalf("Failed waiting for job %q: %v", jobName, err)
		}
		return res
	}
	rerun := func(t *testing.T, jobName string, mode string) {
		req, err := http.NewRequest("POST", fmt.Sprintf("http://localhost/rerun?mode=%v&prowjob=%v", mode, jobName), nil)
		if err != nil {
			t.Fatalf("Could not generate a request %v", err)
		}

		// Deck might not have been informed about the job config update, retry
		// for this case.
		waitDur := time.Second * 5
		var lastErr error
		for i := 0; i < 3; i++ {
			lastErr = nil
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				lastErr = fmt.Errorf("could not make post request %v", err)
				res.Body.Close()
				break
			}
			// The only retry condition is status not ok
			if res.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("status not expected: %d", res.StatusCode)
				res.Body.Close()
				waitDur *= 2
				time.Sleep(waitDur)
				continue
			}
			body, err := io.ReadAll(res.Body)
			if err != nil {
				lastErr = fmt.Errorf("could not read body response %v", err)
				res.Body.Close()
				break
			}
			t.Logf("Response body: %s", string(body))
			break
		}
		if lastErr != nil {
			t.Fatalf("Failed trigger rerun: %v", lastErr)
		}
	}
	jobToRerun := getLatestJob(t, jobName, nil)
	rerunJobConfig = fmt.Sprintf(rerunJobConfigTemplate, jobName, "bar")
	if err := updateJobConfig(context.Background(), kubeClient, rerunJobConfigFile, []byte(rerunJobConfig)); err != nil {
		t.Fatalf("Failed update job config: %v", err)
	}
	var passed bool
	// It may take some time for the new ProwJob to show up, so we will
	// check every 30s interval three times for it to appear
	latestRun := jobToRerun
	for i := 0; i < 3; i++ {
		time.Sleep(30 * time.Second)
		rerun(t, jobToRerun.Name, "latest")
		if latestRun = getLatestJob(t, jobName, &latestRun.CreationTimestamp); latestRun.Labels["foo"] == "bar" {
			passed = true
			break
		}
	}
	if !passed {
		t.Fatal("Expected updated job.")
	}

	// Deck scheduled job from latest configuration, rerun with "original"
	// should still go with original configuration.
	rerun(t, jobToRerun.Name, "original")
	if latestRun := getLatestJob(t, jobName, &latestRun.CreationTimestamp); latestRun.Labels["foo"] != "foo" {
		t.Fatalf("Job label mismatch. Want: 'foo', got: '%s'", latestRun.Labels["foo"])
	}
}

func renamePJs(pjs *prowjobv1.ProwJobList, name string) prowjobv1.ProwJobList {
	res := prowjobv1.ProwJobList{Items: []prowjobv1.ProwJob{}}
	for _, pj := range pjs.Items {
		renamed := pj.DeepCopy()
		renamed.ObjectMeta.Name = pj.ObjectMeta.Name + name
		res.Items = append(res.Items, *renamed)
	}
	return res
}

func expectedPJsInDeck(pjs *prowjobv1.ProwJobList, deck *prowjobv1.ProwJobList) bool {
	for _, expected := range getSpecs(pjs) {
		found := false
		for _, spec := range getSpecs(deck) {
			if diff := cmp.Diff(expected, spec); diff == "" {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func unexpectedPJsInDeck(pjs *prowjobv1.ProwJobList, deck *prowjobv1.ProwJobList) bool {
	for _, unexpected := range getSpecs(pjs) {
		for _, spec := range getSpecs(deck) {
			if diff := cmp.Diff(unexpected, spec); diff == "" {
				return true
			}
		}
	}
	return false
}
