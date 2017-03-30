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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var FAKE_RES = "{\"name\": \"res\", \"type\": \"t\", \"state\": \"s\"}"
var FAKE_MAP = "{\"res\":\"user\"}"

func ErrStr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestStart(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, FAKE_RES)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "user")
	name, err := c.Start("t", "s")
	if err != nil {
		t.Errorf("Error in start : %v", err)
	} else if name != "res" {
		t.Errorf("Got resource name %v, expect res", name)
	} else if len(c.resources) != 1 {
		t.Errorf("Resource in client: %d, expect 1", len(c.resources))
	}
}

func TestDone(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []string
		res       string
		errWanted string
	}{
		{
			name:      "all - no res",
			resources: []string{},
			res:       "",
			errWanted: "No holding resource",
		},
		{
			name:      "one - no res",
			resources: []string{},
			res:       "res",
			errWanted: "No resource name res",
		},
		{
			name:      "one - no match",
			resources: []string{"foo"},
			res:       "res",
			errWanted: "No resource name res",
		},
		{
			name:      "all - ok",
			resources: []string{"foo"},
			res:       "",
			errWanted: "",
		},
		{
			name:      "one - ok",
			resources: []string{"res"},
			res:       "res",
			errWanted: "",
		},
	}

	for _, tc := range testcases {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer ts.Close()

		c := NewClient(ts.URL, "user")
		for _, r := range tc.resources {
			c.resources = append(c.resources, r)
		}
		var err error
		if tc.res == "" {
			err = c.DoneAll("d")
		} else {
			err = c.DoneOne(tc.res, "d")
		}

		if ErrStr(err) != tc.errWanted {
			t.Errorf("Got err %v, expect %v", err, tc.errWanted)
		}

		if tc.errWanted == "" && len(c.resources) != 0 {
			t.Errorf("Resource count %v, expect 0", len(c.resources))
		}
	}
}

func TestUpdate(t *testing.T) {
	var testcases = []struct {
		name      string
		resources []string
		res       string
		errWanted string
	}{
		{
			name:      "all - no res",
			resources: []string{},
			res:       "",
			errWanted: "No holding resource",
		},
		{
			name:      "one - no res",
			resources: []string{},
			res:       "res",
			errWanted: "No resource name res",
		},
		{
			name:      "one - no match",
			resources: []string{"foo"},
			res:       "res",
			errWanted: "No resource name res",
		},
		{
			name:      "all - ok",
			resources: []string{"foo"},
			res:       "",
			errWanted: "",
		},
		{
			name:      "one - ok",
			resources: []string{"res"},
			res:       "res",
			errWanted: "",
		},
	}

	for _, tc := range testcases {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer ts.Close()

		c := NewClient(ts.URL, "user")
		for _, r := range tc.resources {
			c.resources = append(c.resources, r)
		}
		var err error
		if tc.res == "" {
			err = c.UpdateAll()
		} else {
			err = c.UpdateOne(tc.res)
		}

		if ErrStr(err) != tc.errWanted {
			t.Errorf("Got err %v, expect %v", err, tc.errWanted)
		}
	}
}

func TestReset(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, FAKE_MAP)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, "user")
	rmap, err := c.Reset("t", "s", time.Minute, "d")
	if err != nil {
		t.Errorf("Error in reset : %v", err)
	} else if len(rmap) != 1 {
		t.Errorf("Resource in returned map: %d, expect 1", len(c.resources))
	} else if rmap["res"] != "user" {
		t.Errorf("Owner of res: %d, expect user", rmap["res"])
	}
}
