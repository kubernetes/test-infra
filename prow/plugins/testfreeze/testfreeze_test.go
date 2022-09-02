/*
Copyright 2022 The Kubernetes Authors.

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

package testfreeze

import (
	"errors"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"

	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/plugins/testfreeze/checker"
	"k8s.io/test-infra/prow/plugins/testfreeze/testfreezefakes"
)

var errTest = errors.New("")

func TestHandle(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name              string
		action            github.PullRequestEventAction
		org, repo, branch string
		prepare           func(*testfreezefakes.FakeVerifier)
		assert            func(*testfreezefakes.FakeVerifier, error)
	}{
		{
			name:   "success in test freeze",
			action: github.PullRequestActionOpened,
			org:    defaultKubernetesRepoAndOrg,
			repo:   defaultKubernetesRepoAndOrg,
			branch: defaultKubernetesBranch,
			prepare: func(mock *testfreezefakes.FakeVerifier) {
				mock.CheckInTestFreezeReturns(&checker.Result{
					InTestFreeze:    true,
					Tag:             "v1.23.0",
					Branch:          "release-1.23",
					LastFastForward: "Wed May  4 16:15:37 CEST 2022",
				}, nil)
			},
			assert: func(mock *testfreezefakes.FakeVerifier, err error) {
				assert.Nil(t, err)
				assert.Equal(t, 1, mock.CreateCommentCallCount())
				_, _, _, _, comment := mock.CreateCommentArgsForCall(0)
				assert.Contains(t, comment, "Please note that we're already")
				assert.Contains(t, comment, "for the `release-1.23` branch")
				assert.Contains(t, comment, "Fast forwards are scheduled to happen every 6 hours, whereas the most recent run was: Wed May  4 16:15:37 CEST 2022")
			},
		},
		{
			name:   "success not in test freeze",
			action: github.PullRequestActionOpened,
			org:    defaultKubernetesRepoAndOrg,
			repo:   defaultKubernetesRepoAndOrg,
			branch: defaultKubernetesBranch,
			prepare: func(mock *testfreezefakes.FakeVerifier) {
				mock.CheckInTestFreezeReturns(&checker.Result{}, nil)
			},
			assert: func(mock *testfreezefakes.FakeVerifier, err error) {
				assert.Nil(t, err)
				assert.Equal(t, 0, mock.CreateCommentCallCount())
			},
		},
		{
			name:    "success filtered action",
			action:  github.PullRequestActionClosed,
			org:     defaultKubernetesRepoAndOrg,
			repo:    defaultKubernetesRepoAndOrg,
			branch:  defaultKubernetesBranch,
			prepare: func(mock *testfreezefakes.FakeVerifier) {},
			assert: func(mock *testfreezefakes.FakeVerifier, err error) {
				assert.Zero(t, mock.CreateCommentCallCount())
				assert.Nil(t, err)
			},
		},
		{
			name:    "success filtered org",
			action:  github.PullRequestActionOpened,
			org:     "invalid",
			repo:    defaultKubernetesRepoAndOrg,
			branch:  defaultKubernetesBranch,
			prepare: func(mock *testfreezefakes.FakeVerifier) {},
			assert: func(mock *testfreezefakes.FakeVerifier, err error) {
				assert.Zero(t, mock.CreateCommentCallCount())
				assert.Nil(t, err)
			},
		},
		{
			name:    "success filtered repo",
			action:  github.PullRequestActionOpened,
			org:     defaultKubernetesRepoAndOrg,
			repo:    "invalid",
			branch:  defaultKubernetesBranch,
			prepare: func(mock *testfreezefakes.FakeVerifier) {},
			assert: func(mock *testfreezefakes.FakeVerifier, err error) {
				assert.Zero(t, mock.CreateCommentCallCount())
				assert.Nil(t, err)
			},
		},
		{
			name:    "success filtered branch",
			action:  github.PullRequestActionOpened,
			org:     defaultKubernetesRepoAndOrg,
			repo:    defaultKubernetesRepoAndOrg,
			branch:  "invalid",
			prepare: func(mock *testfreezefakes.FakeVerifier) {},
			assert: func(mock *testfreezefakes.FakeVerifier, err error) {
				assert.Zero(t, mock.CreateCommentCallCount())
				assert.Nil(t, err)
			},
		},
		{
			name:   "error CheckInTestFreeze",
			action: github.PullRequestActionOpened,
			org:    defaultKubernetesRepoAndOrg,
			repo:   defaultKubernetesRepoAndOrg,
			branch: defaultKubernetesBranch,
			prepare: func(mock *testfreezefakes.FakeVerifier) {
				mock.CheckInTestFreezeReturns(nil, errTest)
			},
			assert: func(mock *testfreezefakes.FakeVerifier, err error) {
				assert.NotNil(t, err)
			},
		},
		{
			name:   "error CreateComment",
			action: github.PullRequestActionOpened,
			org:    defaultKubernetesRepoAndOrg,
			repo:   defaultKubernetesRepoAndOrg,
			branch: defaultKubernetesBranch,
			prepare: func(mock *testfreezefakes.FakeVerifier) {
				mock.CheckInTestFreezeReturns(&checker.Result{
					InTestFreeze: true,
				}, nil)
				mock.CreateCommentReturns(errTest)
			},
			assert: func(mock *testfreezefakes.FakeVerifier, err error) {
				assert.NotNil(t, err)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mock := &testfreezefakes.FakeVerifier{}
			tc.prepare(mock)

			sut := newHandler()
			sut.verifier = mock

			entry := logrus.NewEntry(logrus.StandardLogger())
			err := sut.handle(entry, nil, tc.action, 0, tc.org, tc.repo, tc.branch)
			tc.assert(mock, err)
		})
	}
}
