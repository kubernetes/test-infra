/*
Copyright 2023 The Kubernetes Authors.

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

package cherrypickapproved

import (
	"errors"
	"regexp"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/labels"
	"k8s.io/test-infra/prow/plugins"
	"k8s.io/test-infra/prow/plugins/cherrypickapproved/cherrypickapprovedfakes"
)

const testOrgRepo = "kubernetes"

var errTest = errors.New("test")

func TestHandle(t *testing.T) {
	t.Parallel()

	successEvent := &github.ReviewEvent{
		Action: github.ReviewActionSubmitted,
		Review: github.Review{
			State: github.ReviewStateApproved,
			User:  github.User{Login: "user"},
		},
		Repo: github.Repo{
			Name:  testOrgRepo,
			Owner: github.User{Login: testOrgRepo},
		},
		PullRequest: github.PullRequest{
			Base: github.PullRequestBranch{Ref: "release-1.29"},
		},
	}

	testConfig := []plugins.CherryPickApproved{
		{
			Org:       testOrgRepo,
			Repo:      testOrgRepo,
			BranchRe:  regexp.MustCompile("^release-*"),
			Approvers: []string{"user"},
		},
	}

	for _, tc := range []struct {
		name        string
		config      []plugins.CherryPickApproved
		modifyEvent func(*github.ReviewEvent) *github.ReviewEvent
		prepare     func(*cherrypickapprovedfakes.FakeImpl)
		assert      func(*cherrypickapprovedfakes.FakeImpl, error)
	}{
		{
			name:   "success apply cherry-pick-approved label",
			config: testConfig,
			prepare: func(mock *cherrypickapprovedfakes.FakeImpl) {
				mock.GetCombinedStatusReturns(&github.CombinedStatus{}, nil)
				mock.GetIssueLabelsReturns(
					[]github.Label{
						{Name: labels.LGTM},
						{Name: labels.Approved},
						{Name: labels.CpUnapproved},
					},
					nil,
				)
			},
			assert: func(mock *cherrypickapprovedfakes.FakeImpl, err error) {
				assert.NoError(t, err)
				assert.EqualValues(t, 1, mock.AddLabelCallCount())
				assert.EqualValues(t, 1, mock.RemoveLabelCallCount())
			},
		},
		{
			name:   "success but failed to apply/remove labels",
			config: testConfig,
			prepare: func(mock *cherrypickapprovedfakes.FakeImpl) {
				mock.GetCombinedStatusReturns(&github.CombinedStatus{}, nil)
				mock.GetIssueLabelsReturns(
					[]github.Label{
						{Name: labels.LGTM},
						{Name: labels.Approved},
						{Name: labels.CpUnapproved},
					},
					nil,
				)
				mock.AddLabelReturns(errTest)
				mock.RemoveLabelReturns(errTest)
			},
			assert: func(mock *cherrypickapprovedfakes.FakeImpl, err error) {
				assert.NoError(t, err)
				assert.EqualValues(t, 1, mock.AddLabelCallCount())
				assert.EqualValues(t, 1, mock.RemoveLabelCallCount())
			},
		},
		{
			name: "skip non approver",
			config: []plugins.CherryPickApproved{{
				Org:       testOrgRepo,
				Repo:      testOrgRepo,
				BranchRe:  regexp.MustCompile("^release-*"),
				Approvers: []string{"wrong"},
			}},
			prepare: func(mock *cherrypickapprovedfakes.FakeImpl) {
				mock.GetCombinedStatusReturns(&github.CombinedStatus{}, nil)
				mock.GetIssueLabelsReturns(
					[]github.Label{
						{Name: labels.LGTM},
						{Name: labels.Approved},
						{Name: labels.CpUnapproved},
					},
					nil,
				)
			},
			assert: func(mock *cherrypickapprovedfakes.FakeImpl, err error) {
				assert.NoError(t, err)
				assert.EqualValues(t, 0, mock.AddLabelCallCount())
				assert.EqualValues(t, 0, mock.RemoveLabelCallCount())
			},
		},
		{
			name:   "error on GetIssueLabels",
			config: testConfig,
			prepare: func(mock *cherrypickapprovedfakes.FakeImpl) {
				mock.GetCombinedStatusReturns(&github.CombinedStatus{}, nil)
				mock.GetIssueLabelsReturns(nil, errTest)
			},
			assert: func(mock *cherrypickapprovedfakes.FakeImpl, err error) {
				assert.Error(t, err)
			},
		},
		{
			name:   "error on GetCombinedStatus",
			config: testConfig,
			prepare: func(mock *cherrypickapprovedfakes.FakeImpl) {
				mock.GetCombinedStatusReturns(nil, errTest)
			},
			assert: func(mock *cherrypickapprovedfakes.FakeImpl, err error) {
				assert.Error(t, err)
			},
		},
		{
			name:   "skip with failed tests",
			config: testConfig,
			prepare: func(mock *cherrypickapprovedfakes.FakeImpl) {
				mock.GetCombinedStatusReturns(&github.CombinedStatus{
					Statuses: []github.Status{{State: github.StatusError}},
				}, nil)
			},
			assert: func(mock *cherrypickapprovedfakes.FakeImpl, err error) {
				assert.NoError(t, err)
				assert.EqualValues(t, 0, mock.GetIssueLabelsCallCount())
			},
		},
		{
			name:   "skip with wrong review state",
			config: testConfig,
			modifyEvent: func(e *github.ReviewEvent) *github.ReviewEvent {
				newEvent := *e
				newEvent.Review.State = github.ReviewStateCommented
				return &newEvent
			},
			prepare: func(mock *cherrypickapprovedfakes.FakeImpl) {},
			assert: func(mock *cherrypickapprovedfakes.FakeImpl, err error) {
				assert.NoError(t, err)
				assert.EqualValues(t, 0, mock.GetCombinedStatusCallCount())
			},
		},
		{
			name:   "skip with wrong review action",
			config: testConfig,
			modifyEvent: func(e *github.ReviewEvent) *github.ReviewEvent {
				newEvent := *e
				newEvent.Action = github.ReviewActionEdited
				return &newEvent
			},
			prepare: func(mock *cherrypickapprovedfakes.FakeImpl) {},
			assert: func(mock *cherrypickapprovedfakes.FakeImpl, err error) {
				assert.NoError(t, err)
				assert.EqualValues(t, 0, mock.GetCombinedStatusCallCount())
			},
		},
		{
			name:   "skip with non matching branch",
			config: testConfig,
			modifyEvent: func(e *github.ReviewEvent) *github.ReviewEvent {
				newEvent := *e
				newEvent.PullRequest.Base.Ref = "invalid"
				return &newEvent
			},
			prepare: func(mock *cherrypickapprovedfakes.FakeImpl) {},
			assert: func(mock *cherrypickapprovedfakes.FakeImpl, err error) {
				assert.NoError(t, err)
				assert.EqualValues(t, 0, mock.GetCombinedStatusCallCount())
			},
		},
		{
			name: "skip with no approvers",
			config: []plugins.CherryPickApproved{{
				Org:      testOrgRepo,
				Repo:     testOrgRepo,
				BranchRe: regexp.MustCompile("^release-*"),
			}},
			prepare: func(mock *cherrypickapprovedfakes.FakeImpl) {},
			assert: func(mock *cherrypickapprovedfakes.FakeImpl, err error) {
				assert.NoError(t, err)
				assert.EqualValues(t, 0, mock.GetCombinedStatusCallCount())
			},
		},
		{
			name:   "skip with non matching repo",
			config: testConfig,
			modifyEvent: func(e *github.ReviewEvent) *github.ReviewEvent {
				newEvent := *e
				newEvent.Repo.Name = "invalid"
				return &newEvent
			},
			prepare: func(mock *cherrypickapprovedfakes.FakeImpl) {},
			assert: func(mock *cherrypickapprovedfakes.FakeImpl, err error) {
				assert.NoError(t, err)
				assert.EqualValues(t, 0, mock.GetCombinedStatusCallCount())
			},
		},
		{
			name:   "skip with non matching org",
			config: testConfig,
			modifyEvent: func(e *github.ReviewEvent) *github.ReviewEvent {
				newEvent := *e
				newEvent.Repo.Owner.Login = "invalid"
				return &newEvent
			},
			prepare: func(mock *cherrypickapprovedfakes.FakeImpl) {},
			assert: func(mock *cherrypickapprovedfakes.FakeImpl, err error) {
				assert.NoError(t, err)
				assert.EqualValues(t, 0, mock.GetCombinedStatusCallCount())
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &cherrypickapprovedfakes.FakeImpl{}
			tc.prepare(mock)

			event := successEvent
			if tc.modifyEvent != nil {
				event = tc.modifyEvent(event)
			}

			sut := newHandler()
			sut.impl = mock

			log := logrus.NewEntry(logrus.StandardLogger())
			err := sut.handle(log, nil, *event, tc.config)

			tc.assert(mock, err)
		})
	}
}
