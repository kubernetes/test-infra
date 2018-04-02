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

package client

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"k8s.io/test-infra/boskos/common"
)

const (
	FakeRes    = "{\"name\": \"res\", \"type\": \"t\", \"state\": \"d\"}"
	FakeMap    = "{\"res\":\"user\"}"
	FakeMetric = "{\"type\":\"t\",\"current\":{\"s\":1},\"owner\":{\"merlin\":1}}"
)

func AreErrorsEqual(got error, expect error) bool {
	if got == nil && expect == nil {
		return true
	}

	if got == nil || expect == nil {
		return false
	}

	return got.Error() == expect.Error()
}

func TestAcquire(t *testing.T) {
	var testcases = []struct {
		name      string
		serverErr bool
		expectErr error
	}{
		{
			name:      "request error",
			serverErr: true,
			expectErr: fmt.Errorf("status %d %s, status code %d", http.StatusBadRequest, http.StatusText(http.StatusBadRequest), http.StatusBadRequest),
		},
		{
			name:      "request successful",
			expectErr: nil,
		},
	}

	for _, tc := range testcases {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tc.serverErr {
				http.Error(w, "", http.StatusBadRequest)
			} else {
				fmt.Fprint(w, FakeRes)
			}
		}))
		defer ts.Close()

		c := NewClient("user", ts.URL)
		res, err := c.Acquire("t", "s", "d")

		if !AreErrorsEqual(err, tc.expectErr) {
			t.Errorf("Test %v, got error %v, expect error %v", tc.name, err, tc.expectErr)
		}
		if err == nil {
			if res.Name != "res" {
				t.Errorf("Test %v, got resource name %v, expect res", tc.name, res.Name)
			} else if len(c.resources) != 1 {
				t.Errorf("Test %v, resource in client: %d, expect 1", tc.name, len(c.resources))
			}
		}
	}
}

func TestRelease(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []string
		res       string
		expectErr error
	}{
		{
			name:      "all - no res",
			resources: []string{},
			res:       "",
			expectErr: errors.New("no holding resource"),
		},
		{
			name:      "one - no res",
			resources: []string{},
			res:       "res",
			expectErr: errors.New("no resource name res"),
		},
		{
			name:      "one - no match",
			resources: []string{"foo"},
			res:       "res",
			expectErr: errors.New("no resource name res"),
		},
		{
			name:      "all - ok",
			resources: []string{"foo"},
			res:       "",
			expectErr: nil,
		},
		{
			name:      "one - ok",
			resources: []string{"res"},
			res:       "res",
			expectErr: nil,
		},
	}

	for _, tc := range testcases {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer ts.Close()

		c := NewClient("user", ts.URL)
		c.resources = append(c.resources, tc.resources...)
		var err error
		if tc.res == "" {
			err = c.ReleaseAll("d")
		} else {
			err = c.ReleaseOne(tc.res, "d")
		}

		if !AreErrorsEqual(err, tc.expectErr) {
			t.Errorf("Test %v, got err %v, expect %v", tc.name, err, tc.expectErr)
		}

		if tc.expectErr == nil && len(c.resources) != 0 {
			t.Errorf("Test %v, resource count %v, expect 0", tc.name, len(c.resources))
		}
	}
}

func TestUpdate(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []string
		res       string
		expectErr error
	}{
		{
			name:      "all - no res",
			resources: []string{},
			res:       "",
			expectErr: errors.New("no holding resource"),
		},
		{
			name:      "one - no res",
			resources: []string{},
			res:       "res",
			expectErr: errors.New("no resource name res"),
		},
		{
			name:      "one - no match",
			resources: []string{"foo"},
			res:       "res",
			expectErr: errors.New("no resource name res"),
		},
		{
			name:      "all - ok",
			resources: []string{"foo"},
			res:       "",
			expectErr: nil,
		},
		{
			name:      "one - ok",
			resources: []string{"res"},
			res:       "res",
			expectErr: nil,
		},
	}

	for _, tc := range testcases {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer ts.Close()
		c := NewClient("user", ts.URL)
		c.resources = append(c.resources, tc.resources...)
		var err error
		if tc.res == "" {
			err = c.UpdateAll("s")
		} else {
			err = c.UpdateOne(tc.res, "s", nil)
		}

		if !AreErrorsEqual(err, tc.expectErr) {
			t.Errorf("Test %v, got err %v, expect %v", tc.name, err, tc.expectErr)
		}
	}
}

func TestReset(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, FakeMap)
	}))
	defer ts.Close()

	c := NewClient("user", ts.URL)
	rmap, err := c.Reset("t", "s", time.Minute, "d")
	if err != nil {
		t.Errorf("Error in reset : %v", err)
	} else if len(rmap) != 1 {
		t.Errorf("Resource in returned map: %d, expect 1", len(c.resources))
	} else if rmap["res"] != "user" {
		t.Errorf("Owner of res: %s, expect user", rmap["res"])
	}
}

func TestMetric(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, FakeMetric)
	}))
	defer ts.Close()
	expectMetric := common.Metric{
		Type: "t",
		Current: map[string]int{
			"s": 1,
		},
		Owners: map[string]int{
			"merlin": 1,
		},
	}

	c := NewClient("user", ts.URL)
	metric, err := c.Metric("t")
	if err != nil {
		t.Errorf("Error in reset : %v", err)
	} else if !reflect.DeepEqual(metric, expectMetric) {
		t.Errorf("wrong metric, got %v, want %v", metric, expectMetric)
	}
}
