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
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	prowjobv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
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
	body, err := ioutil.ReadAll(resp.Body)
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
			body, err := ioutil.ReadAll(resp.Body)
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
