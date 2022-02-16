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
	"testing"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"k8s.io/test-infra/prow/plugins/approve/testfreeze/testfreezefakes"
)

func TestInTestFreeze(t *testing.T) {
	t.Parallel()

	releaseBranch := func(v string) *plumbing.Reference {
		return plumbing.NewReferenceFromStrings("refs/heads/release-"+v, "")
	}

	tag := func(v string) *plumbing.Reference {
		return plumbing.NewReferenceFromStrings("refs/tags/"+v, "")
	}

	for _, tc := range []struct {
		prepare func(*testfreezefakes.FakeImpl)
		assert  func(*Result, error)
	}{
		{ // success no test freez
			prepare: func(mock *testfreezefakes.FakeImpl) {
				mock.ListRefsReturns([]*plumbing.Reference{
					tag("wrong"),       // unable to parse this tag, but don't error
					releaseBranch("1"), // unable to parse this branch, but don't error
					releaseBranch("1.18"),
					tag("1.18.0"),
					releaseBranch("1.23"),
					tag("1.23.0"),
					releaseBranch("1.22"),
					tag("1.22.0"),
				}, nil)
			},
			assert: func(res *Result, err error) {
				assert.False(t, res.InTestFreeze)
				assert.Equal(t, "release-1.23", res.Branch)
				assert.Equal(t, "v1.23.0", res.Tag)
				assert.Nil(t, err)
			},
		},
		{ // success in test freeze
			prepare: func(mock *testfreezefakes.FakeImpl) {
				mock.ListRefsReturns([]*plumbing.Reference{
					releaseBranch("1.18"),
					releaseBranch("1.24"),
					releaseBranch("1.22"),
					tag("1.18.0"),
					tag("1.22.0"),
				}, nil)
			},
			assert: func(res *Result, err error) {
				assert.True(t, res.InTestFreeze)
				assert.Equal(t, "release-1.24", res.Branch)
				assert.Equal(t, "v1.24.0", res.Tag)
				assert.Nil(t, err)
			},
		},
		{ // error no latest releae branch found
			prepare: func(mock *testfreezefakes.FakeImpl) {
				mock.ListRefsReturns([]*plumbing.Reference{
					tag("1.22.0"),
				}, nil)
			},
			assert: func(res *Result, err error) {
				assert.Nil(t, res)
				assert.NotNil(t, err)
			},
		},
		{ // error on list refs
			prepare: func(mock *testfreezefakes.FakeImpl) {
				mock.ListRefsReturns(nil, errors.New(""))
			},
			assert: func(res *Result, err error) {
				assert.Nil(t, res)
				assert.NotNil(t, err)
			},
		},
	} {
		mock := &testfreezefakes.FakeImpl{}
		tc.prepare(mock)

		sut := NewChecker(logrus.NewEntry(logrus.StandardLogger()))
		sut.impl = mock

		res, err := sut.InTestFreeze()

		tc.assert(res, err)
	}
}
