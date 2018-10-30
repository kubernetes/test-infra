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

package main

import (
	"net/url"
	"testing"
)

func TestJobHistURL(t *testing.T) {
	cases := []struct {
		name    string
		address string
		bktName string
		root    string
		id      int64
		expErr  bool
	}{
		{
			address: "http://www.example.com/job-history/foo-bucket/logs/bar-e2e",
			bktName: "foo-bucket",
			root:    "logs/bar-e2e",
			id:      emptyID,
		},
		{
			address: "http://www.example.com/job-history/foo-bucket/logs/bar-e2e?buildId=",
			bktName: "foo-bucket",
			root:    "logs/bar-e2e",
			id:      emptyID,
		},
		{
			address: "http://www.example.com/job-history/foo-bucket/logs/bar-e2e?buildId=123456789123456789",
			bktName: "foo-bucket",
			root:    "logs/bar-e2e",
			id:      123456789123456789,
		},
		{
			address: "http://www.example.com/job-history/foo-bucket",
			expErr:  true,
		},
		{
			address: "http://www.example.com/job-history/foo-bucket/",
			expErr:  true,
		},
		{
			address: "http://www.example.com/job-history/foo-bucket/logs/bar-e2e?buildId=-738",
			expErr:  true,
		},
		{
			address: "http://www.example.com/job-history/foo-bucket/logs/bar-e2e?buildId=nope",
			expErr:  true,
		},
	}
	for _, tc := range cases {
		u, _ := url.Parse(tc.address)
		bktName, root, id, err := jobHistURL(u)
		if tc.expErr {
			if err == nil && tc.expErr {
				t.Errorf("parsing %q: expected error", tc.address)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsing %q: unexpected error: %v", tc.address, err)
		}
		if bktName != tc.bktName {
			t.Errorf("parsing %q: expected bucket %s, got %s", tc.address, tc.bktName, bktName)
		}
		if root != tc.root {
			t.Errorf("parsing %q: expected root %s, got %s", tc.address, tc.root, root)
		}
		if id != tc.id {
			t.Errorf("parsing %q: expected id %d, got %d", tc.address, tc.id, id)
		}
	}
}

func eq(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCropResults(t *testing.T) {
	cases := []struct {
		a   []int64
		max int64
		exp []int64
		p   int
		q   int
	}{
		{
			a:   []int64{},
			max: 42,
			exp: []int64{},
			p:   -1,
			q:   0,
		},
		{
			a:   []int64{81, 27, 9, 3, 1},
			max: 100,
			exp: []int64{81, 27, 9, 3, 1},
			p:   0,
			q:   4,
		},
		{
			a:   []int64{81, 27, 9, 3, 1},
			max: 50,
			exp: []int64{27, 9, 3, 1},
			p:   1,
			q:   4,
		},
		{
			a:   []int64{25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
			max: 23,
			exp: []int64{23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4},
			p:   2,
			q:   21,
		},
	}
	for _, tc := range cases {
		actual, firstIndex, lastIndex := cropResults(tc.a, tc.max)
		if !eq(actual, tc.exp) || firstIndex != tc.p || lastIndex != tc.q {
			t.Errorf("cropResults(%v, %d) expected (%v, %d, %d), got (%v, %d, %d)",
				tc.a, tc.max, tc.exp, tc.p, tc.q, actual, firstIndex, lastIndex)
		}
	}
}

func TestLinkID(t *testing.T) {
	cases := []struct {
		startAddr string
		id        int64
		expAddr   string
	}{
		{
			startAddr: "http://www.example.com/job-history/foo-bucket/logs/bar-e2e",
			id:        -1,
			expAddr:   "http://www.example.com/job-history/foo-bucket/logs/bar-e2e?buildId=",
		},
		{
			startAddr: "http://www.example.com/job-history/foo-bucket/logs/bar-e2e",
			id:        23,
			expAddr:   "http://www.example.com/job-history/foo-bucket/logs/bar-e2e?buildId=23",
		},
	}
	for _, tc := range cases {
		u, _ := url.Parse(tc.startAddr)
		actual := linkID(u, tc.id)
		if actual != tc.expAddr {
			t.Errorf("adding id param %d expected %s, got %s", tc.id, tc.expAddr, actual)
		}
		again, _ := url.Parse(tc.startAddr)
		if again.String() != u.String() {
			t.Errorf("linkID incorrectly mutated URL (expected %s, got %s)", u.String(), again.String())
		}
	}
}
