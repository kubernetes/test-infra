/*
Copyright 2017 The Kubernetes Authors.

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

package gcs

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"cloud.google.com/go/storage"
)

func TestUploadToGcs(t *testing.T) {
	var testCases = []struct {
		name           string
		passingTargets int
		failingTargets int
		expectedErr    bool
	}{
		{
			name:           "all passing",
			passingTargets: 10,
			failingTargets: 0,
			expectedErr:    false,
		},
		{
			name:           "all but one passing",
			passingTargets: 10,
			failingTargets: 1,
			expectedErr:    true,
		},
		{
			name:           "all but one failing",
			passingTargets: 1,
			failingTargets: 10,
			expectedErr:    true,
		},
		{
			name:           "all failing",
			passingTargets: 0,
			failingTargets: 10,
			expectedErr:    true,
		},
	}

	for _, testCase := range testCases {
		lock := sync.Mutex{}
		count := 0

		update := func() {
			lock.Lock()
			defer lock.Unlock()
			count = count + 1
		}

		fail := func(_ dataWriter) error {
			update()
			return errors.New("fail")
		}

		success := func(_ dataWriter) error {
			update()
			return nil
		}

		targets := map[string]UploadFunc{}
		for i := 0; i < testCase.passingTargets; i++ {
			targets[fmt.Sprintf("pass-%d", i)] = success
		}

		for i := 0; i < testCase.failingTargets; i++ {
			targets[fmt.Sprintf("fail-%d", i)] = fail
		}

		err := Upload(&storage.BucketHandle{}, targets)
		if err != nil && !testCase.expectedErr {
			t.Errorf("%s: expected no error but got %v", testCase.name, err)
		}
		if err == nil && testCase.expectedErr {
			t.Errorf("%s: expected an error but got none", testCase.name)
		}

		if count != (testCase.passingTargets + testCase.failingTargets) {
			t.Errorf("%s: had %d passing and %d failing targets but only ran %d targets, not %d", testCase.name, testCase.passingTargets, testCase.failingTargets, count, testCase.passingTargets+testCase.failingTargets)
		}
	}
}
