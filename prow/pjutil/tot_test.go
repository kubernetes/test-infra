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

package pjutil

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type responseVendor struct {
	codes []int
	data  []string

	position int
}

func (r *responseVendor) next() (int, string) {
	code := r.codes[r.position]
	datum := r.data[r.position]

	r.position = r.position + 1
	if r.position == len(r.codes) {
		r.position = 0
	}

	return code, datum
}

func parrotServer(codes []int, data []string) *httptest.Server {
	vendor := responseVendor{
		codes: codes,
		data:  data,
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, datum := vendor.next()
		w.WriteHeader(code)
		fmt.Fprint(w, datum)
	}))
}

func TestGetBuildID(t *testing.T) {
	oldSleep := sleep
	sleep = func(time.Duration) {}
	defer func() { sleep = oldSleep }()

	var testCases = []struct {
		name        string
		codes       []int
		data        []string
		expected    string
		expectedErr bool
	}{
		{
			name:        "all good",
			codes:       []int{200},
			data:        []string{"yay"},
			expected:    "yay",
			expectedErr: false,
		},
		{
			name:        "fail then success",
			codes:       []int{500, 200},
			data:        []string{"boo", "yay"},
			expected:    "yay",
			expectedErr: false,
		},
		{
			name:        "fail",
			codes:       []int{500},
			data:        []string{"boo"},
			expected:    "boo",
			expectedErr: true,
		},
	}

	for _, testCase := range testCases {
		totServ := parrotServer(testCase.codes, testCase.data)

		actual, actualErr := GetBuildID("dummy", totServ.URL)
		if testCase.expectedErr && actualErr == nil {
			t.Errorf("%s: expected an error but got none", testCase.name)
		} else if !testCase.expectedErr && actualErr != nil {
			t.Errorf("%s: expected no error but got one: %v", testCase.name, actualErr)
		} else if !testCase.expectedErr && actual != testCase.expected {
			t.Errorf("%s: expected response %v but got: %v", testCase.name, testCase.expected, actual)
		}

		totServ.Close()
	}
}
