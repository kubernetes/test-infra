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
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andygrunwald/go-gerrit"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/diff"
	fakectrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/crier/reporters/criercommonlib"
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
	count         int
}

func (f *fgc) SetReview(instance, id, revision, message string, labels map[string]string) error {
	if instance != f.instance {
		return fmt.Errorf("wrong instance: %s", instance)
	}
	exist, err := f.ChangeExist(instance, id)
	if err != nil {
		return err
	}
	if !exist {
		return errors.New("change not exist: 404")
	}
	change, err := f.GetChange(instance, id)
	if err != nil {
		return err
	}

	if _, ok := change.Revisions[revision]; !ok {
		return errors.New("revision doesn't exist")
	}

	for label := range labels {
		if label == "bad-label" {
			return fmt.Errorf("bad label")
		}
	}
	f.reportMessage = message
	if len(labels) > 0 {
		f.reportLabel = labels
	}
	f.count++
	return nil
}

func (f *fgc) GetChange(instance, id string, addtionalFields ...string) (*gerrit.ChangeInfo, error) {
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

func (f *fgc) ChangeExist(instance, id string) (bool, error) {
	if f.changes == nil {
		return false, errors.New("fake client changes is not initialized")
	}
	changes, ok := f.changes[instance]
	if !ok {
		return false, fmt.Errorf("instance %s not found", instance)
	}
	for _, change := range changes {
		if change.ID == id {
			return true, nil
		}
	}
	return false, nil
}

func TestReport(t *testing.T) {
	changes := map[string][]*gerrit.ChangeInfo{
		"gerrit": {
			{ID: "123-abc", Status: "NEW", Revisions: map[string]gerrit.RevisionInfo{"abc": {}}},
			{ID: "merged", Status: "MERGED", Revisions: map[string]gerrit.RevisionInfo{"abc": {}}},
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
		expectError       bool
		numExpectedReport int
	}{
		{
			name: "1 job, unfinished, should not report",
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Report: true,
				},
				Status: v1.ProwJobStatus{
					State: v1.PendingState,
				},
			},
		},
		{
			name: "1 job, finished, no labels, should not report",
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Report: true,
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
				},
			},
		},
		{
			name: "1 job, finished, missing gerrit-id label, should not report",
			pj: &v1.ProwJob{
				Spec: v1.ProwJobSpec{
					Report: true,
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritInstance: "gerrit",
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
				Spec: v1.ProwJobSpec{
					Report: true,
				},
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						kube.GerritID:          "123-abc",
						kube.GerritInstance:    "gerrit",
						kube.GerritReportLabel: "Code-Review",
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
				Spec: v1.ProwJobSpec{
					Report: true,
				},
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID: "123-abc",
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
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{codeReview: lgtm},
			numExpectedReport: 0,
		},
		{
			name: "1-job-passed-change-missing",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-not-exist",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			expectReport:      true,
			numExpectedReport: 0,
		},
		{
			name: "1-job-passed-revision-missing",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "not-exist",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			expectReport:      true,
			numExpectedReport: 0,
		},
		{
			name: "1 job, passed, skip report set true, should not report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: false,
				},
			},
		},
		{
			name: "1 job, passed, bad label, should report without label",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "bad-label",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			numExpectedReport: 0,
		},
		{
			name: "1 job, passed, empty label, should report, but not vote",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			numExpectedReport: 0,
		},
		{
			name: "1 job, ABORTED, should not report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.AbortedState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			expectReport: false,
		},
		{
			name: "1 job, passed, with customized label, should report to customized label",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.GerritReportLabel: "foobar-label",
						kube.ProwJobTypeLabel:  presubmit,
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{"foobar-label": lgtm},
			numExpectedReport: 0,
		},
		{
			name: "1 job, failed, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.FailureState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			expectReport:      true,
			reportInclude:     []string{"0 out of 1", "ci-foo", "FAILURE", "guber/foo"},
			expectLabel:       map[string]string{codeReview: lbtm},
			numExpectedReport: 0,
		},
		{
			name: "1 job, passed, has slash in repo name, should report and handle slash properly",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo/bar",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo/bar",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo-bar",
					Report: true,
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo-bar", "SUCCESS", "guber/foo/bar"},
			reportExclude:     []string{"foo_bar"},
			expectLabel:       map[string]string{codeReview: lgtm},
			numExpectedReport: 0,
		},
		{
			name: "1 job, no pulls, should error",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
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
					Job:    "ci-foo",
					Report: true,
				},
			},
			expectReport: true,
			expectError:  true,
		},
		{
			name: "1 postsubmit job, no pulls, should error",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  postsubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			expectReport: true,
			expectError:  true,
		},
		{
			name: "2 jobs, one passed, other job finished but on different revision, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "def",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "Code-Review",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-def",
							kube.GerritInstance: "gerrit",
						},
						Name:      "ci-foo",
						Namespace: "test-pods",
					},
					Status: v1.ProwJobStatus{
						State: v1.SuccessState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-bar",
						Report: true,
					},
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			reportExclude:     []string{"2", "bar"},
			expectLabel:       map[string]string{codeReview: lgtm},
			numExpectedReport: 0,
		},
		{
			name: "2 jobs, one passed, other job unfinished with same label, should not report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "Code-Review",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
						},
					},
					Status: v1.ProwJobStatus{
						State: v1.PendingState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-bar",
						Report: true,
					},
				},
			},
		},
		{
			name: "2 jobs, 1 passed, 1 pending, empty labels, should not wait for aggregation, no vote",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
						},
						Name:      "ci-foo",
						Namespace: "test-pods",
					},
					Status: v1.ProwJobStatus{
						State: v1.PendingState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-bar",
						Report: true,
					},
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			reportExclude:     []string{"2", "bar"},
			numExpectedReport: 0,
		},
		{
			name: "non-presubmit failures vote zero",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  postsubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.FailureState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PostsubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			expectReport:      true,
			expectLabel:       map[string]string{codeReview: lztm},
			numExpectedReport: 0,
		},
		{
			name: "2 jobs, one passed, other job failed, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "Code-Review",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
						},
						Name:      "ci-bar",
						Namespace: "test-pods",
					},
					Status: v1.ProwJobStatus{
						State: v1.FailureState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Type: v1.PresubmitJob,
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-bar",
						Report: true,
					},
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 2", "ci-foo", "SUCCESS", "ci-bar", "FAILURE", "guber/foo", "guber/bar"},
			reportExclude:     []string{"0", "2 out of 2"},
			expectLabel:       map[string]string{codeReview: lbtm},
			numExpectedReport: 0,
		},
		{
			name: "2 jobs, both passed, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "Code-Review",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
						},
						Name:      "ci-bar",
						Namespace: "test-pods",
					},
					Status: v1.ProwJobStatus{
						State: v1.SuccessState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-bar",
						Report: true,
					},
				},
			},
			expectReport:      true,
			reportInclude:     []string{"2 out of 2", "ci-foo", "SUCCESS", "ci-bar", "guber/foo", "guber/bar"},
			reportExclude:     []string{"1", "0", "FAILURE"},
			expectLabel:       map[string]string{codeReview: lgtm},
			numExpectedReport: 0,
		},
		{
			name: "2 jobs, one passed, one aborted, should report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Type:   v1.PresubmitJob,
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "Code-Review",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
						},
						Name:      "ci-bar",
						Namespace: "test-pods",
					},
					Status: v1.ProwJobStatus{
						State: v1.AbortedState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-bar",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 2", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{codeReview: lbtm},
			numExpectedReport: 0,
		},
		{
			name: "postsubmit after presubmit on same revision, should report separately",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.GerritReportLabel: "postsubmit-label",
						kube.ProwJobTypeLabel:  postsubmit,
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "Code-Review",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
						},
						Name:      "ci-bar",
						Namespace: "test-pods",
					},
					Status: v1.ProwJobStatus{
						State: v1.SuccessState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-bar",
						Report: true,
					},
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{"postsubmit-label": lgtm},
			numExpectedReport: 0,
		},
		{
			name: "2 jobs, both passed, different label, should report by itself",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "label-foo",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "label-bar",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
						},
						Name:      "ci-foo",
						Namespace: "test-pods",
					},
					Status: v1.ProwJobStatus{
						State: v1.SuccessState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-bar",
						Report: true,
					},
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{"label-foo": lgtm},
			numExpectedReport: 0,
		},
		{
			name: "one job, reported, retriggered, should report by itself",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "label-foo",
						kube.OrgLabel:          "org",
						kube.RepoLabel:         "repo",
						kube.PullLabel:         "0",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow,
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "label-foo",
							kube.OrgLabel:          "org",
							kube.RepoLabel:         "repo",
							kube.PullLabel:         "0",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Minute),
						},
						Name:      "ci-foo",
						Namespace: "test-pods",
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
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-foo",
						Report: true,
					},
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{"label-foo": lgtm},
			numExpectedReport: 0,
		},
		{
			name: "older job, should not report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "label-foo",
						kube.OrgLabel:          "org",
						kube.RepoLabel:         "repo",
						kube.PullLabel:         "0",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow.Add(-2 * time.Minute),
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "label-foo",
							kube.OrgLabel:          "org",
							kube.RepoLabel:         "repo",
							kube.PullLabel:         "0",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Minute),
						},
						Name:      "ci-foo",
						Namespace: "test-pods",
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
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-foo",
						Report: true,
					},
				},
			},
		},
		{
			name: "2 jobs, one SUCCESS one pending, different label, should report by itself",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "label-foo",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "label-bar",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
						},
						Name:      "ci-bar",
						Namespace: "test-pods",
					},
					Status: v1.ProwJobStatus{
						State: v1.PendingState,
						URL:   "guber/bar",
					},
					Spec: v1.ProwJobSpec{
						Refs: &v1.Refs{
							Repo: "bar",
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-bar",
						Report: true,
					},
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{"label-foo": lgtm},
			numExpectedReport: 0,
		},
		{
			name: "2 jobs, both failed, already reported, same label, retrigger one and passed, should report both and not lgtm",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "same-label",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
					},
					CreationTimestamp: metav1.Time{
						Time: timeNow,
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "same-label",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
						Name:      "ci-bar",
						Namespace: "test-pods",
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
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-bar",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "same-label",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
						},
						CreationTimestamp: metav1.Time{
							Time: timeNow.Add(-time.Hour),
						},
						Name:      "ci-foo",
						Namespace: "test-pods",
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
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-foo",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 2", "ci-foo", "SUCCESS", "ci-bar", "FAILURE", "guber/foo", "guber/bar", "Comment `/retest`"},
			expectLabel:       map[string]string{"same-label": lbtm},
			numExpectedReport: 0,
		},
		{
			name: "2 jobs, both failed, job from newer patchset pending, should not report",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "same-label",
						kube.GerritPatchset:    "5",
						kube.OrgLabel:          "same-org",
						kube.RepoLabel:         "same-repo",
						kube.PullLabel:         "123456",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
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
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "same-label",
							kube.GerritPatchset:    "5",
							kube.OrgLabel:          "same-org",
							kube.RepoLabel:         "same-repo",
							kube.PullLabel:         "123456",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
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
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-bar",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "same-label",
							kube.GerritPatchset:    "5",
							kube.OrgLabel:          "same-org",
							kube.RepoLabel:         "same-repo",
							kube.PullLabel:         "123456",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
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
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-foo",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "def",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "same-label",
							kube.GerritPatchset:    "6",
							kube.OrgLabel:          "same-org",
							kube.RepoLabel:         "same-repo",
							kube.PullLabel:         "123456",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-def",
							kube.GerritInstance: "gerrit",
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
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-foo",
						Type:   v1.PresubmitJob,
						Report: true,
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
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "same-label",
						kube.GerritPatchset:    "5",
						kube.OrgLabel:          "same-org",
						kube.RepoLabel:         "same-repo",
						kube.PullLabel:         "123456",
					},
					Annotations: map[string]string{
						kube.GerritID:       "123-abc",
						kube.GerritInstance: "gerrit",
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
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			existingPJs: []*v1.ProwJob{
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "same-label",
							kube.GerritPatchset:    "5",
							kube.OrgLabel:          "same-org",
							kube.RepoLabel:         "same-repo",
							kube.PullLabel:         "123456",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
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
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-bar",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "abc",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "same-label",
							kube.GerritPatchset:    "5",
							kube.OrgLabel:          "same-org",
							kube.RepoLabel:         "same-repo",
							kube.PullLabel:         "123456",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-abc",
							kube.GerritInstance: "gerrit",
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
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-foo",
						Type:   v1.PresubmitJob,
						Report: true,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							kube.GerritRevision:    "def",
							kube.ProwJobTypeLabel:  presubmit,
							kube.GerritReportLabel: "same-label",
							kube.GerritPatchset:    "6",
							kube.OrgLabel:          "same-org",
							kube.RepoLabel:         "same-repo",
							kube.PullLabel:         "123456",
						},
						Annotations: map[string]string{
							kube.GerritID:       "123-def",
							kube.GerritInstance: "gerrit",
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
							Pulls: []v1.Pull{
								{
									Number: 0,
								},
							},
						},
						Job:    "ci-foo",
						Type:   v1.PresubmitJob,
						Report: true,
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
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "merged",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.FailureState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Type: v1.PresubmitJob,
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			expectReport:      true,
			reportInclude:     []string{"0 out of 1", "ci-foo", "FAILURE", "guber/foo", "Comment `/retest`"},
			expectLabel:       map[string]string{codeReview: lztm},
			numExpectedReport: 0,
		},
		{
			name: "1 job, passed, should vote +1 even after merge",
			pj: &v1.ProwJob{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						kube.GerritRevision:    "abc",
						kube.ProwJobTypeLabel:  presubmit,
						kube.GerritReportLabel: "Code-Review",
					},
					Annotations: map[string]string{
						kube.GerritID:       "merged",
						kube.GerritInstance: "gerrit",
					},
					Name:      "ci-foo",
					Namespace: "test-pods",
				},
				Status: v1.ProwJobStatus{
					State: v1.SuccessState,
					URL:   "guber/foo",
				},
				Spec: v1.ProwJobSpec{
					Refs: &v1.Refs{
						Repo: "foo",
						Pulls: []v1.Pull{
							{
								Number: 0,
							},
						},
					},
					Job:    "ci-foo",
					Report: true,
				},
			},
			expectReport:      true,
			reportInclude:     []string{"1 out of 1", "ci-foo", "SUCCESS", "guber/foo"},
			expectLabel:       map[string]string{codeReview: lgtm},
			numExpectedReport: 0,
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

			reporter := &Client{
				gc:          fgc,
				pjclientset: fakectrlruntimeclient.NewFakeClient(allpj...),
				prLocks:     criercommonlib.NewShardedLock(),
			}

			shouldReport := reporter.ShouldReport(context.Background(), logrus.NewEntry(logrus.StandardLogger()), tc.pj)
			if shouldReport != tc.expectReport {
				t.Errorf("shouldReport: %v, expectReport: %v", shouldReport, tc.expectReport)
			}

			if !shouldReport {
				return
			}

			reportedJobs, _, err := reporter.Report(context.Background(), logrus.NewEntry(logrus.StandardLogger()), tc.pj)
			if err != nil {
				if !tc.expectError {
					t.Errorf("Unexpected error: %v", err)
				}
				// if this error is expected then no need to verify anything
				// later
				return
			}

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
		})
	}
}

func TestMultipleWorks(t *testing.T) {
	samplePJ := v1.ProwJob{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				kube.GerritRevision:    "abc",
				kube.ProwJobTypeLabel:  presubmit,
				kube.GerritReportLabel: "same-label",
				kube.GerritPatchset:    "5",
				kube.OrgLabel:          "same-org",
				kube.RepoLabel:         "same-repo",
				kube.PullLabel:         "123456",
			},
			Annotations: map[string]string{
				kube.GerritID:       "123-abc",
				kube.GerritInstance: "gerrit",
			},
			CreationTimestamp: metav1.Time{
				Time: timeNow.Add(-time.Hour),
			},
		},
		Status: v1.ProwJobStatus{
			State: v1.FailureState,
			URL:   "guber/bar",
		},
		Spec: v1.ProwJobSpec{
			Refs: &v1.Refs{
				Repo: "bar",
				Pulls: []v1.Pull{
					{
						Number: 0,
					},
				},
			},
			Job:    "ci-bar",
			Type:   v1.PresubmitJob,
			Report: true,
		},
	}

	// Running with 3 different batches to increase the chance of hitting races
	for _, count := range []int{10, 20, 30} {
		t.Run(fmt.Sprintf("%d-jobs", count), func(t *testing.T) {
			expectedCount := 1
			expectedComment := []string{" out of " + strconv.Itoa(count), "ci-bar", "FAILURE", "guber/bar", "Comment `/retest`"}
			var existingPJs []*v1.ProwJob
			for i := 0; i < count; i++ {
				pj := samplePJ.DeepCopy()
				pj.Spec.Job += strconv.Itoa(i)
				if i%2 == 0 {
					pj.Status.State = v1.SuccessState
				}
				existingPJs = append(existingPJs, pj)
			}

			changes := map[string][]*gerrit.ChangeInfo{
				"gerrit": {
					{ID: "123-abc", Status: "NEW", Revisions: map[string]gerrit.RevisionInfo{"abc": {}}},
				},
			}

			fgc := &fgc{instance: "gerrit", changes: changes}
			var allpj []runtime.Object
			for idx, pj := range existingPJs {
				pj.Name = strconv.Itoa(idx)
				allpj = append(allpj, pj)
			}

			reporter := &Client{
				gc:          fgc,
				pjclientset: fakectrlruntimeclient.NewFakeClient(allpj...),
				prLocks:     criercommonlib.NewShardedLock(),
			}

			g := new(errgroup.Group)
			resChan := make(chan []*v1.ProwJob, count)
			for _, pj := range existingPJs {
				pj := pj.DeepCopy()
				g.Go(func() error {
					toReportJobs, _, err := reporter.Report(context.Background(), logrus.NewEntry(logrus.StandardLogger()), pj)
					if err != nil {
						return err
					}
					resChan <- toReportJobs
					return nil
				})
			}

			if err := g.Wait(); err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if expectedCount != fgc.count {
				t.Fatalf("Expect comment count: %d, got: %d", expectedCount, fgc.count)
			}
			for _, expect := range expectedComment {
				if !strings.Contains(fgc.reportMessage, expect) {
					t.Fatalf("Expect comment contains %q, got: %q", expect, fgc.reportMessage)
				}
			}

			var reported bool
			for i := 0; i < count; i++ {
				toReportJobs := <-resChan
				if reported && len(toReportJobs) > 0 {
					t.Fatalf("These jobs were already reported, should omit reporting again.")
				}
				if len(toReportJobs) > 0 {
					reported = true
				}
			}

			// Ensure that the statues were reported
			var pjs v1.ProwJobList
			if err := reporter.pjclientset.List(context.Background(), &pjs); err != nil {
				t.Fatalf("Failed listing prowjobs: %v", err)
			}
			if want, got := count, len(pjs.Items); want != got {
				t.Fatalf("Number of prowjobs mismatch. Want: %d, got: %d", want, got)
			}
			for _, pj := range pjs.Items {
				if pj.Status.PrevReportStates == nil {
					t.Fatalf("PrevReportStates should have been set")
				}
				if _, ok := pj.Status.PrevReportStates["gerrit-reporter"]; !ok {
					t.Fatalf("PrevReportStates should have been set. Got: %v", pj.Status.PrevReportStates)
				}
			}
		})
	}
}

func TestJobReportFormats(t *testing.T) {
	tests := []struct {
		name        string
		format      string
		words       []interface{}
		formatRegex string
	}{
		{"jobReportFormat", jobReportFormat, []interface{}{"a", "b", "c", "d"}, jobReportFormatRegex},
		{"jobReportFormatUrlNotFound", jobReportFormatUrlNotFound, []interface{}{"a", "b", "c"}, jobReportFormatUrlNotFoundRegex},
		{"jobReportFormatWithoutURL", jobReportFormatWithoutURL, []interface{}{"a", "b", "c"}, jobReportFormatWithoutURLRegex},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// In GenerateReport(), we use a trailing newline in the
			// jobReportFormat* constants, because we use a newline as a
			// delimiter. In ParseReport(), we split the overall report on
			// newlines first, before applying the jobReportFormat*Regex
			// regexes on them. To mimic this behavior, we trim the newline
			// before attempting to parse them with tc.formatRegex.
			serialized := fmt.Sprintf(tc.format, tc.words...)
			serializedWithoutNewline := strings.TrimSuffix(serialized, "\n")
			re := regexp.MustCompile(tc.formatRegex)
			if !re.MatchString(serializedWithoutNewline) {
				t.Fatalf("could not parse serialized job report line %q with regex %q", serializedWithoutNewline, tc.formatRegex)
			}
		})
	}

	// Ensure the legacy job reporting format can be parsed by
	// jobReportFormatLegacyRegex.
	serializedWithoutNewline := "✔️ some-job SUCCESS - https://someURL.com/somewhere"
	re := regexp.MustCompile(jobReportFormatLegacyRegex)
	if !re.MatchString(serializedWithoutNewline) {
		t.Fatalf("could not parse serialized job report line %q with regex %q", serializedWithoutNewline, jobReportFormatLegacyRegex)
	}
}

func TestGenerateReport(t *testing.T) {
	job := func(name, url string, state v1.ProwJobState, createdByTide bool) *v1.ProwJob {
		var out v1.ProwJob
		out.Spec.Job = name
		out.Status.URL = url
		out.Status.State = state
		out.Labels = make(map[string]string)
		if createdByTide {
			out.Labels[kube.CreatedByTideLabel] = "true"
		}
		return &out
	}

	tests := []struct {
		name             string
		jobs             []*v1.ProwJob
		commentSizeLimit int
		wantHeader       string
		wantMessage      string
	}{
		{
			name: "basic",
			jobs: []*v1.ProwJob{
				job("this", "url", v1.SuccessState, false),
				job("that", "hey", v1.FailureState, false),
				job("left", "foo", v1.AbortedState, false),
				job("right", "bar", v1.ErrorState, false),
			},
			wantHeader:  "Prow Status: 1 out of 4 pjs passed! 👉 Comment `/retest` to rerun only failed tests (if any), or `/test all` to rerun all tests.\n",
			wantMessage: "❌ [that](hey) FAILURE\n🚫 [right](bar) ERROR\n🚫 [left](foo) ABORTED\n✔️ [this](url) SUCCESS\n",
		},
		{
			name: "include-tide-jobs",
			jobs: []*v1.ProwJob{
				job("this", "url", v1.SuccessState, true),
				job("that", "hey", v1.FailureState, false),
				job("left", "foo", v1.AbortedState, false),
				job("right", "bar", v1.ErrorState, false),
			},
			wantHeader:  "Prow Status: 1 out of 4 pjs passed! 👉 Comment `/retest` to rerun only failed tests (if any), or `/test all` to rerun all tests. (Not a duplicated report. Some of the jobs below were triggered by Tide)\n",
			wantMessage: "❌ [that](hey) FAILURE\n🚫 [right](bar) ERROR\n🚫 [left](foo) ABORTED\n✔️ [this](url) SUCCESS\n",
		},
		{
			name: "short lines only",
			jobs: []*v1.ProwJob{
				job("this", "url", v1.SuccessState, false),
				job("that", "hey", v1.FailureState, false),
				job("some", "other", v1.SuccessState, false),
			},
			// 131 is the length of the Header.
			// 154 is the comment size room we give for the Message part. Note
			// that it should be 1 char more than what we have in the
			// wantMessage part, because we always return comments *under* the
			// commentSizeLimit.
			commentSizeLimit: 131 + 154,
			wantHeader:       "Prow Status: 2 out of 3 pjs passed! 👉 Comment `/retest` to rerun only failed tests (if any), or `/test all` to rerun all tests.\n",
			wantMessage:      "❌ that FAILURE\n✔️ some SUCCESS\n✔️ this SUCCESS\n[NOTE FROM PROW: Skipped displaying URLs for 3/3 jobs due to reaching gerrit comment size limit]",
		},
		{
			name: "mix of short and long lines",
			jobs: []*v1.ProwJob{
				job("this", "url", v1.SuccessState, false),
				job("that", "hey", v1.FailureState, false),
				job("some", "other", v1.SuccessState, false),
			},
			commentSizeLimit: 131 + 161,
			wantHeader:       "Prow Status: 2 out of 3 pjs passed! 👉 Comment `/retest` to rerun only failed tests (if any), or `/test all` to rerun all tests.\n",
			wantMessage:      "❌ [that](hey) FAILURE\n✔️ some SUCCESS\n✔️ this SUCCESS\n[NOTE FROM PROW: Skipped displaying URLs for 2/3 jobs due to reaching gerrit comment size limit]",
		},
		{
			name: "too many jobs",
			jobs: []*v1.ProwJob{
				job("this", "url", v1.SuccessState, false),
				job("that", "hey", v1.FailureState, false),
				job("some", "other", v1.SuccessState, false),
			},
			commentSizeLimit: 1,
			wantHeader:       "Prow Status: 2 out of 3 pjs passed! 👉 Comment `/retest` to rerun only failed tests (if any), or `/test all` to rerun all tests.\n",
			wantMessage:      "[NOTE FROM PROW: Skipped displaying 3/3 jobs due to reaching gerrit comment size limit (too many jobs)]",
		},
		{
			name: "too many jobs; only truncate the last job",
			jobs: []*v1.ProwJob{
				job("this", "url", v1.SuccessState, false),
				job("that", "hey", v1.FailureState, false),
				job("some", "other", v1.SuccessState, false),
			},
			commentSizeLimit: 130 + 150,
			wantHeader:       "Prow Status: 2 out of 3 pjs passed! 👉 Comment `/retest` to rerun only failed tests (if any), or `/test all` to rerun all tests.\n",
			wantMessage:      "❌ that FAILURE\n✔️ some SUCCESS\n[NOTE FROM PROW: Skipped displaying 1/3 jobs due to reaching gerrit comment size limit (too many jobs)]",
		},
		{
			// Check cases where the job could legitimately not have its URL
			// field set (because the job did not even get scheduled).
			name: "missing URLs",
			jobs: []*v1.ProwJob{
				job("right", "", v1.ErrorState, false),
			},
			commentSizeLimit: 1000,
			wantHeader:       "Prow Status: 0 out of 1 pjs passed! 👉 Comment `/retest` to rerun only failed tests (if any), or `/test all` to rerun all tests.\n",
			wantMessage:      "🚫 right (URL_NOT_FOUND) ERROR\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotReport := GenerateReport(tc.jobs, tc.commentSizeLimit)
			if want, got := tc.wantHeader, gotReport.Header; want != got {
				t.Fatalf("Header mismatch. Want:\n%s,\ngot: \n%s", want, got)
			}
			if want, got := tc.wantMessage, gotReport.Message; want != got {
				t.Fatalf("Message mismatch. Want:\n%s\ngot: \n%s", want, got)
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
		// These tests all test the legacy format.
		{
			name:         "parse multiple jobs",
			comment:      "Prow Status: 0 out of 2 passed\n❌️ foo-job FAILURE - http://foo-status\n❌ bar-job FAILURE - http://bar-status",
			expectedJobs: 2,
		},
		{
			name:         "parse job without URL",
			comment:      "Prow Status: 0 out of 2 passed\n❌️ foo-job FAILURE\n❌ bar-job FAILURE",
			expectedJobs: 2,
		},
		{
			name:         "parse mixed formats",
			comment:      "Prow Status: 0 out of 2 passed\n❌️ foo-job FAILURE - http://foo-status\n❌ bar-job FAILURE\n[Skipped displaying URLs for 1/2 jobs due to reaching gerrit comment size limit]",
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
		// New Markdown format (link uses Markdown syntax).
		{
			name:         "parse multiple jobs (Markdown)",
			comment:      "Prow Status: 0 out of 2 passed\n❌️ [foo-job](http://foo-status) FAILURE\n❌ [bar-job](http://bar-status) FAILURE",
			expectedJobs: 2,
		},
		{
			name:         "parse mixed formats (Markdown)",
			comment:      "Prow Status: 0 out of 2 passed\n❌️ [foo-job](http://foo-status) FAILURE\n❌ bar-job FAILURE\n[Skipped displaying URLs for 1/2 jobs due to reaching gerrit comment size limit]",
			expectedJobs: 2,
		},
		{
			name:         "parse one job (Markdown)",
			comment:      "Prow Status: 0 out of 1 passed\n❌ [bar-job](http://bar-status) FAILURE",
			expectedJobs: 1,
		},
		{
			name:      "do not parse without the header (Markdown)",
			comment:   "0 out of 1 passed\n❌ [bar-job](http://bar-status) FAILURE",
			expectNil: true,
		},
		{
			name: "parse with extra stuff at the start as long as the header and jobs start on new lines (Markdown)",
			comment: `qwerty
Patch Set 1:
Prow Status: 0 out of 2 pjs passed!
❌ [foo-job](https://foo-status) FAILURE
❌ [bar-job](https://bar-status) FAILURE
`,
			expectedJobs: 2,
		},
		{
			name:         "invalid job state (Markdown)",
			comment:      "Prow Status: 0 out of 1 passed\n❌ [bar-job](http://bar-status) BANANAS",
			expectedJobs: 0,
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
		job("this", "hey", v1.SuccessState),
		job("that", "url", v1.FailureState),
	}, 0)
	actual := ParseReport(expected.String())
	if !equality.Semantic.DeepEqual(&expected, actual) {
		t.Errorf(diff.ObjectReflectDiff(&expected, actual))
	}
}
