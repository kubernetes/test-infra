/*
Copyright 2018 The Kubernetes Authors.

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

package reporter

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	pjlister "k8s.io/test-infra/prow/client/listers/prowjobs/v1"
	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/kube"
)

type fgc struct {
	reportMessage string
	reportLabel   map[string]string
	instance      string
}

func (f *fgc) SetReview(instance, id, revision, message string, labels map[string]string) error {
	if instance != f.instance {
		return fmt.Errorf("wrong instance: %s", instance)
	}
	f.reportMessage = message
	f.reportLabel = labels
	return nil
}

type fakeLister struct {
	pjs []*v1.ProwJob
}

func (fl fakeLister) List(selector labels.Selector) (ret []*v1.ProwJob, err error) {
	result := []*v1.ProwJob{}
	for _, pj := range fl.pjs {
		if selector.Matches(labels.Set(pj.ObjectMeta.Labels)) {
			result = append(result, pj)
		}
	}

	return result, nil
}

func (fl fakeLister) ProwJobs(namespace string) pjlister.ProwJobNamespaceLister {
	return nil
}

func TestReport(t *testing.T) {

	var testcases = []struct {
		name          string
		pj            *v1.ProwJob
		existingPJs   []*v1.ProwJob
		expectReport  bool
		reportInclude []string
		reportExclude []string
		expectLabel   map[string]string
	}{
		{
			name: "1 job, unfinished, should not report",
			pj: &v1.ProwJob{
				Status: v1.ProwJobStatus{
					State: v1.PendingState,
				},
			},
		},
		{
			name: "1 job, finished, no labels, should not report",
			pj: &v1.ProwJob{
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
		},
		{
			name: "1 job, finished, missing gerrit-id label, should not report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision: "abc",
						kube.ProwJobTypeLabel: "presubmit",
					},
					Annotations: map[string]string{
						client.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
		},
		{
			name: "1 job, finished, missing gerrit-revision label, should not report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
		},
		{
			name: "1 job, finished, missing gerrit-instance label, should not report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision: "abc",
						kube.ProwJobTypeLabel: "presubmit",
					},
					Annotations: map[string]string{
						client.GerritID: "123-abc",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
		},
		{
			name: "1 job, passed, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision: "abc",
						kube.ProwJobTypeLabel: "presubmit",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job: "ci-foo",
				},
			},
			expectReport:  true,
			reportInclude: []string{"1 out of 1", "ci-foo", "success", "guber/foo"},
			expectLabel:   map[string]string{client.CodeReview: client.LGTM},
		},
		{
			name: "1 job, passed, with customized label, should report to customized label",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						client.GerritReportLabel: "foobar-label",
						kube.ProwJobTypeLabel:    "presubmit",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job: "ci-foo",
				},
			},
			expectReport:  true,
			reportInclude: []string{"1 out of 1", "ci-foo", "success", "guber/foo"},
			expectLabel:   map[string]string{"foobar-label": client.LGTM},
		},
		{
			name: "1 job, failed, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision: "abc",
						kube.ProwJobTypeLabel: "presubmit",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.FailureState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job: "ci-foo",
				},
			},
			expectReport:  true,
			reportInclude: []string{"0 out of 1", "ci-foo", "failure", "guber/foo"},
			expectLabel:   map[string]string{client.CodeReview: client.LBTM},
		},
		{
			name: "1 job, passed, has slash in repo name, should report and handle slash properly",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision: "abc",
						kube.ProwJobTypeLabel: "presubmit",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo/bar",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo/bar",
					},
					Job: "ci-foo-bar",
				},
			},
			expectReport:  true,
			reportInclude: []string{"1 out of 1", "ci-foo-bar", "success", "guber/foo/bar"},
			reportExclude: []string{"foo_bar"},
			expectLabel:   map[string]string{client.CodeReview: client.LGTM},
		},
		{
			name: "2 jobs, one passed, other job finished but on different revision, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision: "abc",
						kube.ProwJobTypeLabel: "presubmit",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job: "ci-foo",
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							client.GerritRevision: "def",
							kube.ProwJobTypeLabel: "presubmit",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-def",
							client.GerritInstance: "gerrit",
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.SuccessState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
						},
						Job: "ci-bar",
					},
				},
			},
			expectReport:  true,
			reportInclude: []string{"1 out of 1", "ci-foo", "success", "guber/foo"},
			reportExclude: []string{"2", "bar"},
			expectLabel:   map[string]string{client.CodeReview: client.LGTM},
		},
		{
			name: "2 jobs, one passed, other job unfinished, should not report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision: "abc",
						kube.ProwJobTypeLabel: "presubmit",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job: "ci-foo",
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							client.GerritRevision: "abc",
							kube.ProwJobTypeLabel: "presubmit",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-abc",
							client.GerritInstance: "gerrit",
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.PendingState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
						},
						Job: "ci-bar",
					},
				},
			},
		},
		{
			name: "2 jobs, one passed, other job failed, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision: "abc",
						kube.ProwJobTypeLabel: "presubmit",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job: "ci-foo",
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							client.GerritRevision: "abc",
							kube.ProwJobTypeLabel: "presubmit",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-abc",
							client.GerritInstance: "gerrit",
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
						},
						Job: "ci-bar",
					},
				},
			},
			expectReport:  true,
			reportInclude: []string{"1 out of 2", "ci-foo", "success", "ci-bar", "failure", "guber/foo", "guber/bar"},
			reportExclude: []string{"0", "2 out of 2"},
			expectLabel:   map[string]string{client.CodeReview: client.LBTM},
		},
		{
			name: "2 jobs, both passed, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision: "abc",
						kube.ProwJobTypeLabel: "presubmit",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job: "ci-foo",
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							client.GerritRevision: "abc",
							kube.ProwJobTypeLabel: "presubmit",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-abc",
							client.GerritInstance: "gerrit",
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.SuccessState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
						},
						Job: "ci-bar",
					},
				},
			},
			expectReport:  true,
			reportInclude: []string{"2 out of 2", "ci-foo", "success", "ci-bar", "guber/foo", "guber/bar"},
			reportExclude: []string{"1", "0", "failure"},
			expectLabel:   map[string]string{client.CodeReview: client.LGTM},
		},
		{
			name: "postsubmit after presubmit on same revision, should report separately",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						client.GerritReportLabel: "postsubmit-label",
						kube.ProwJobTypeLabel:    "postsubmit",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job: "ci-foo",
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							client.GerritRevision: "abc",
							kube.ProwJobTypeLabel: "presubmit",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-abc",
							client.GerritInstance: "gerrit",
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.SuccessState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
						},
						Job: "ci-bar",
					},
				},
			},
			expectReport:  true,
			reportInclude: []string{"1 out of 1", "ci-foo", "success", "guber/foo"},
			expectLabel:   map[string]string{"postsubmit-label": client.LGTM},
		},
	}

	for _, tc := range testcases {
		fgc := &fgc{instance: "gerrit"}
		allpj := []*v1.ProwJob{tc.pj}
		if tc.existingPJs != nil {
			allpj = append(allpj, tc.existingPJs...)
		}

		fl := &fakeLister{pjs: allpj}
		reporter := &Client{gc: fgc, lister: fl}

		shouldReport := reporter.ShouldReport(tc.pj)
		if shouldReport != tc.expectReport {
			t.Errorf("test: %s: shouldReport: %v, expectReport: %v", tc.name, shouldReport, tc.expectReport)
		}

		if !shouldReport {
			continue
		}

		err := reporter.Report(tc.pj)
		if err != nil {
			t.Errorf("test: %s: expect no error but got error %v", tc.name, err)
		}

		if err == nil {
			for _, include := range tc.reportInclude {
				if !strings.Contains(fgc.reportMessage, include) {
					t.Errorf("test: %s: reported with: %s, should contain: %s", tc.name, fgc.reportMessage, include)
				}
			}
			for _, exclude := range tc.reportExclude {
				if strings.Contains(fgc.reportMessage, exclude) {
					t.Errorf("test: %s: reported with: %s, should not contain: %s", tc.name, fgc.reportMessage, exclude)
				}
			}

			if !reflect.DeepEqual(tc.expectLabel, fgc.reportLabel) {
				t.Errorf("test: %s: reported with %s label, should have %s label", tc.name, fgc.reportLabel, tc.expectLabel)
			}
		}
	}
}
