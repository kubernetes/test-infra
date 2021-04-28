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

package gerrit

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/gerrit/client"
	"k8s.io/test-infra/prow/kube"
)

var timeNow = time.Date(1234, time.May, 15, 1, 2, 3, 4, time.UTC)

const (
	presubmit  = string(v1.PresubmitJob)
	postsubmit = string(v1.PostsubmitJob)
)

type fgc struct {
	reportMessage string
	reportLabel   map[string]string
	instance      string
	changes       map[string][]*gerrit.ChangeInfo
}

func (f *fgc) SetReview(instance, id, revision, message string, labels map[string]string) error {
	if instance != f.instance {
		return fmt.Errorf("wrong instance: %s", instance)
	}
	for label := range labels {
		if label == "bad-label" {
			return fmt.Errorf("bad label")
		}
	}
	f.reportMessage = message
	f.reportLabel = labels
	return nil
}

func (f *fgc) GetChange(instance, id string) (*gerrit.ChangeInfo, error) {
	if f.changes == nil {
		return nil, errors.New("fake client changes is not initialized")
	}
	changes, ok := f.changes[instance]
	if !ok {
		return nil, fmt.Errorf("instance %s not found", instance)
	}
	for _, change := range changes {
		if change.ID == id {
			return change, nil
		}
	}
	return nil, nil
}

func TestReport(t *testing.T) {
	changes := map[string][]*gerrit.ChangeInfo{
		"gerrit": {
			{ID: "123-abc", Status: "NEW"},
			{ID: "merged", Status: "MERGED"},
		},
	}
	var testcases = []struct {
		name              string
		pj                *v1.ProwJob
		existingPJs       []*v1.ProwJob
		expectReport      bool
		reportInclude     []string
		reportExclude     []string
		expectLabel       map[string]string
		numExpectedReport int
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
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "Code-Review",
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
						client.GerritID:          "123-abc",
						client.GerritInstance:    "gerrit",
						client.GerritReportLabel: "Code-Review",
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
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "Code-Review",
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
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "Code-Review",
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
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{codeReview: lgtm},
			numExpectedReport: 1,
		},
		{
			name: "1 job, passed, bad label, should report without label",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "bad-label",
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
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			numExpectedReport: 1,
		},
		{
			name: "1 job, passed, empty label, should report, but not vote",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "",
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
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			numExpectedReport: 1,
		},
		{
			name: "1 job, ABORTED, should not report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.AbortedState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job: "ci-foo",
				},
			},
			expectReport: false,
		},
		{
			name: "1 job, passed, with customized label, should report to customized label",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						client.GerritReportLabel: "foobar-label",
						kube.ProwJobTypeLabel:    presubmit,
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
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{"foobar-label": lgtm},
			numExpectedReport: 1,
		},
		{
			name: "1 job, failed, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "Code-Review",
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
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job: "ci-foo",
				},
			},
			expectReport:      true,
			reportInclude:     []string{"0 out of 1", "ci-foo", "FAILURE", "guber/foo"},
			expectLabel:       map[string]string{codeReview: lbtm},
			numExpectedReport: 1,
		},
		{
			name: "1 job, passed, has slash in repo name, should report and handle slash properly",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "Code-Review",
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
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo-bar", "SUCCESS", "guber/foo/bar"},
			reportExclude:     []string{"foo_bar"},
			expectLabel:       map[string]string{codeReview: lgtm},
			numExpectedReport: 1,
		},
		{
			name: "2 jobs, one passed, other job finished but on different revision, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "Code-Review",
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
							client.GerritRevision:    "def",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "Code-Review",
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
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			reportExclude:     []string{"2", "bar"},
			expectLabel:       map[string]string{codeReview: lgtm},
			numExpectedReport: 1,
		},
		{
			name: "2 jobs, one passed, other job unfinished with same label, should not report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "Code-Review",
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
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "Code-Review",
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
			name: "2 jobs, 1 passed, 1 pending, empty labels, should not wait for aggregation, no vote",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "",
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
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "",
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
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			reportExclude:     []string{"2", "bar"},
			numExpectedReport: 1,
		},
		{
			name: "non-presubmit failures vote zero",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    postsubmit,
						client.GerritReportLabel: "Code-Review",
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
					Type: v1.PostsubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job: "ci-foo",
				},
			},
			expectReport:      true,
			expectLabel:       map[string]string{codeReview: lztm},
			numExpectedReport: 1,
		},
		{
			name: "2 jobs, one passed, other job failed, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "Code-Review",
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
					Type: v1.PresubmitJob,
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
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "Code-Review",
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
						Type: v1.PresubmitJob,
						Refs: &v1.Refs{
							Repo: "bar",
						},
						Job: "ci-bar",
					},
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 2", "ci-foo", "SUCCESS", "ci-bar", "FAILURE", "guber/foo", "guber/bar"},
			reportExclude:     []string{"0", "2 out of 2"},
			expectLabel:       map[string]string{codeReview: lbtm},
			numExpectedReport: 2,
		},
		{
			name: "2 jobs, both passed, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "Code-Review",
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
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "Code-Review",
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
			expectReport:      true,
			reportInclude:     []string{"2 out of 2", "ci-foo", "SUCCESS", "ci-bar", "guber/foo", "guber/bar"},
			reportExclude:     []string{"1", "0", "FAILURE"},
			expectLabel:       map[string]string{codeReview: lgtm},
			numExpectedReport: 2,
		},
		{
			name: "2 jobs, one passed, one aborted, should report but skip aborted job",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "Code-Review",
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
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "Code-Review",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-abc",
							client.GerritInstance: "gerrit",
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.AbortedState,
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
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			reportExclude:     []string{"2", "0", "FAILURE", "ABORTED", "ci-bar", "guber/bar"},
			expectLabel:       map[string]string{codeReview: lgtm},
			numExpectedReport: 1,
		},
		{
			name: "postsubmit after presubmit on same revision, should report separately",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						client.GerritReportLabel: "postsubmit-label",
						kube.ProwJobTypeLabel:    postsubmit,
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
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "Code-Review",
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
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{"postsubmit-label": lgtm},
			numExpectedReport: 1,
		},
		{
			name: "2 jobs, both passed, different label, should report by itself",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "label-foo",
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
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "label-bar",
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
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{"label-foo": lgtm},
			numExpectedReport: 1,
		},
		{
			name: "one job, reported, retriggered, should report by itself",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "label-foo",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow,
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
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "label-foo",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-abc",
							client.GerritInstance: "gerrit",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Minute),
						},
					},
					Status: v1.ProwJobStatus{
						PrevReportStates: map[string]v1.ProwJobState{
							"gerrit-reporter": v1.FailureState,
						},
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
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{"label-foo": lgtm},
			numExpectedReport: 1,
		},
		{
			name: "2 jobs, one SUCCESS one pending, different label, should report by itself",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "label-foo",
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
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "label-bar",
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
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{"label-foo": lgtm},
			numExpectedReport: 1,
		},
		{
			name: "2 jobs, both failed, already reported, same label, retrigger one and passed, should report both and not lgtm",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "same-label",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow,
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
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
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "same-label",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-abc",
							client.GerritInstance: "gerrit",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						URL:   "guber/bar",
						PrevReportStates: map[string]v1.ProwJobState{
							"gerrit-reporter": v1.FailureState,
						},
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
						},
						Job:  "ci-bar",
						Type: v1.PresubmitJob,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "same-label",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-abc",
							client.GerritInstance: "gerrit",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						URL:   "guber/foo",
						PrevReportStates: map[string]v1.ProwJobState{
							"gerrit-reporter": v1.FailureState,
						},
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "foo",
						},
						Job:  "ci-foo",
						Type: v1.PresubmitJob,
					},
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 2", "ci-foo", "SUCCESS", "ci-bar", "FAILURE", "guber/foo", "guber/bar"},
			expectLabel:       map[string]string{"same-label": lbtm},
			numExpectedReport: 2,
		},
		{
			name: "2 jobs, both failed, job from newer patchset pending, should not report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "same-label",
						client.GerritPatchset:    "5",
						kube.OrgLabel:            "same-org",
						kube.RepoLabel:           "same-repo",
						kube.PullLabel:           "123456",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow,
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
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
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "same-label",
							client.GerritPatchset:    "5",
							kube.OrgLabel:            "same-org",
							kube.RepoLabel:           "same-repo",
							kube.PullLabel:           "123456",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-abc",
							client.GerritInstance: "gerrit",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						URL:   "guber/bar",
						PrevReportStates: map[string]v1.ProwJobState{
							"gerrit-reporter": v1.FailureState,
						},
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
						},
						Job:  "ci-bar",
						Type: v1.PresubmitJob,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "same-label",
							client.GerritPatchset:    "5",
							kube.OrgLabel:            "same-org",
							kube.RepoLabel:           "same-repo",
							kube.PullLabel:           "123456",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-abc",
							client.GerritInstance: "gerrit",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						URL:   "guber/foo",
						PrevReportStates: map[string]v1.ProwJobState{
							"gerrit-reporter": v1.FailureState,
						},
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "foo",
						},
						Job:  "ci-foo",
						Type: v1.PresubmitJob,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							client.GerritRevision:    "def",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "same-label",
							client.GerritPatchset:    "6",
							kube.OrgLabel:            "same-org",
							kube.RepoLabel:           "same-repo",
							kube.PullLabel:           "123456",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-def",
							client.GerritInstance: "gerrit",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.PendingState,
						URL:   "guber/foo",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "foo",
						},
						Job:  "ci-foo",
						Type: v1.PresubmitJob,
					},
				},
			},
			expectReport: false,
		},
		{
			name: "2 jobs, both failed, job from newer patchset failed, should not report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "same-label",
						client.GerritPatchset:    "5",
						kube.OrgLabel:            "same-org",
						kube.RepoLabel:           "same-repo",
						kube.PullLabel:           "123456",
					},
					Annotations: map[string]string{
						client.GerritID:       "123-abc",
						client.GerritInstance: "gerrit",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow,
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
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
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "same-label",
							client.GerritPatchset:    "5",
							kube.OrgLabel:            "same-org",
							kube.RepoLabel:           "same-repo",
							kube.PullLabel:           "123456",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-abc",
							client.GerritInstance: "gerrit",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						URL:   "guber/bar",
						PrevReportStates: map[string]v1.ProwJobState{
							"gerrit-reporter": v1.FailureState,
						},
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
						},
						Job:  "ci-bar",
						Type: v1.PresubmitJob,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							client.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "same-label",
							client.GerritPatchset:    "5",
							kube.OrgLabel:            "same-org",
							kube.RepoLabel:           "same-repo",
							kube.PullLabel:           "123456",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-abc",
							client.GerritInstance: "gerrit",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						URL:   "guber/foo",
						PrevReportStates: map[string]v1.ProwJobState{
							"gerrit-reporter": v1.FailureState,
						},
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "foo",
						},
						Job:  "ci-foo",
						Type: v1.PresubmitJob,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							client.GerritRevision:    "def",
							kube.ProwJobTypeLabel:    presubmit,
							client.GerritReportLabel: "same-label",
							client.GerritPatchset:    "6",
							kube.OrgLabel:            "same-org",
							kube.RepoLabel:           "same-repo",
							kube.PullLabel:           "123456",
						},
						Annotations: map[string]string{
							client.GerritID:       "123-def",
							client.GerritInstance: "gerrit",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
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
						Job:  "ci-foo",
						Type: v1.PresubmitJob,
					},
				},
			},
			expectReport: false,
		},
		{
			name: "1 job, failed after merge, should report with non negative vote",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						client.GerritID:       "merged",
						client.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.FailureState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job: "ci-foo",
				},
			},
			expectReport:      true,
			reportInclude:     []string{"0 out of 1", "ci-foo", "FAILURE", "guber/foo"},
			expectLabel:       map[string]string{codeReview: lztm},
			numExpectedReport: 1,
		},
		{
			name: "1 job, passed, should vote +1 even after merge",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						client.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:    presubmit,
						client.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						client.GerritID:       "merged",
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
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{codeReview: lgtm},
			numExpectedReport: 1,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fgc := &fgc{instance: "gerrit", changes: changes}
			allpj := []runtime.Object{tc.pj}
			for idx, pj := range tc.existingPJs {
				pj.Name = strconv.Itoa(idx)
				allpj = append(allpj, pj)
			}

			reporter := &Client{gc: fgc, lister: fakectrlruntimeclient.NewFakeClient(allpj...)}

			shouldReport := reporter.ShouldReport(context.Background(), logrus.NewEntry(logrus.StandardLogger()), tc.pj)
			if shouldReport != tc.expectReport {
				t.Errorf("shouldReport: %v, expectReport: %v", shouldReport, tc.expectReport)
			}

			if !shouldReport {
				return
			}

			reportedJobs, _, err := reporter.Report(context.Background(), logrus.NewEntry(logrus.StandardLogger()), tc.pj)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if err == nil {
				for _, include := range tc.reportInclude {
					if !strings.Contains(fgc.reportMessage, include) {
						t.Errorf("message: got %q, does not contain %s", fgc.reportMessage, include)
					}
				}
				for _, exclude := range tc.reportExclude {
					if strings.Contains(fgc.reportMessage, exclude) {
						t.Errorf("message: got %q, unexpectedly contains %s", fgc.reportMessage, exclude)
					}
				}

				if !reflect.DeepEqual(tc.expectLabel, fgc.reportLabel) {
					t.Errorf("labels: got %v, want %v", fgc.reportLabel, tc.expectLabel)
				}
				if len(reportedJobs) != tc.numExpectedReport {
					t.Errorf("report count: got %d, want %d", len(reportedJobs), tc.numExpectedReport)
				}
			}
		})
	}
}

func TestParseReport(t *testing.T) {
	var testcases = []struct {
		name         string
		comment      string
		expectedJobs int
		expectNil    bool
	}{
		{
			name:         "parse multiple jobs",
			comment:      "Prow Status: 0 out of 2 passed\n❌️ foo-job FAILURE - http://foo-status\n❌ bar-job FAILURE - http://bar-status",
			expectedJobs: 2,
		},
		{
			name:         "parse one job",
			comment:      "Prow Status: 0 out of 1 passed\n❌ bar-job FAILURE - http://bar-status",
			expectedJobs: 1,
		},
		{
			name:         "parse 0 jobs",
			comment:      "Prow Status: ",
			expectedJobs: 0,
		},
		{
			name:      "do not parse without the header",
			comment:   "0 out of 1 passed\n❌ bar-job FAILURE - http://bar-status",
			expectNil: true,
		},
		{
			name:      "do not parse empty string",
			comment:   "",
			expectNil: true,
		},
		{
			name: "parse with extra stuff at the start as long as the header and jobs start on new lines",
			comment: `qwerty
Patch Set 1:
Prow Status: 0 out of 2 pjs passed!
❌ foo-job FAILURE - https://foo-status
❌ bar-job FAILURE - https://bar-status
`,
			expectedJobs: 2,
		},
	}
	for _, tc := range testcases {
		report := ParseReport(tc.comment)
		if report == nil {
			if !tc.expectNil {
				t.Errorf("%s: expected non-nil report but got nil", tc.name)
			}
		} else {
			if tc.expectNil {
				t.Errorf("%s: expected nil report but got %v", tc.name, report)
			} else if tc.expectedJobs != len(report.Jobs) {
				t.Errorf("%s: expected %d jobs in the report but got %d instead", tc.name, tc.expectedJobs, len(report.Jobs))
			}
		}
	}

}

// TestReportStability ensures a generated report's string parses to the same report
func TestReportStability(t *testing.T) {
	job := func(name, url string, state v1.ProwJobState) *v1.ProwJob {
		var out v1.ProwJob
		out.Spec.Job = name
		out.Status.URL = url
		out.Status.State = state
		return &out
	}
	expected := GenerateReport([]*v1.ProwJob{
		job("this", "url", v1.SuccessState),
		job("that", "hey", v1.FailureState),
	})
	actual := ParseReport(expected.String())
	if !equality.Semantic.DeepEqual(&expected, actual) {
		t.Errorf(diff.ObjectReflectDiff(&expected, actual))
	}
}
