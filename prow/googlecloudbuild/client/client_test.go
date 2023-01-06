/*
Copyright 2021 The Kubernetes Authors.

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
	"testing"

	"github.com/google/go-cmp/cmp"
	cloudbuildpb "google.golang.org/genproto/googleapis/devtools/cloudbuild/v1"
)

func TestProwLabel(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  string
		want string
	}{
		{
			name: "empty",
			want: " ::: ",
		},
		{
			name: "empty-key",
			val:  "aaa",
			want: " ::: aaa",
		},
		{
			name: "empty-val",
			key:  "aaa",
			want: "aaa ::: ",
		},
		{
			name: "normal",
			key:  "aaa",
			val:  "aaa",
			want: "aaa ::: aaa",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got, want := ProwLabel(tc.key, tc.val), tc.want; got != want {
				t.Errorf("want: %s, got: %s", want, got)
			}
		})
	}
}

func TestKvPairFromProwLabel(t *testing.T) {
	tests := []struct {
		name string
		tag  string
		want []string
	}{
		{
			name: "empty",
			tag:  " ::: ",
			want: []string{"", ""},
		},
		{
			name: "empty-key",
			tag:  " ::: aaa",
			want: []string{"", "aaa"},
		},
		{
			name: "empty-val",
			tag:  "aaa ::: ",
			want: []string{"aaa", ""},
		},
		{
			name: "normal",
			tag:  "aaa ::: aaa",
			want: []string{"aaa", "aaa"},
		},
		{
			name: "invalid-separator",
			tag:  "aaa ;;; aaa",
			want: []string{"aaa ;;; aaa", ""},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotKey, gotVal := KvPairFromProwLabel(tc.tag)
			wantKey, wantVal := tc.want[0], tc.want[1]
			if gotKey != wantKey {
				t.Errorf("key mismatching. want: %s, got: %s", wantKey, gotKey)
			}
			if gotVal != wantVal {
				t.Errorf("val mismatching. want: %s, got: %s", wantVal, gotVal)
			}
		})
	}
}

func TestGetProwLabel(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want map[string]string
	}{
		{
			name: "single-label",
			tags: []string{"aaa ::: aaa"},
			want: map[string]string{"aaa": "aaa"},
		},
		{
			name: "multiple-label",
			tags: []string{"aaa ::: aaa", "bbb ::: bbb"},
			want: map[string]string{"aaa": "aaa", "bbb": "bbb"},
		},
		{
			name: "no-label",
			tags: []string{},
			want: map[string]string{},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bld := &cloudbuildpb.Build{
				Tags: tc.tags,
			}
			got := GetProwLabels(bld)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("got(+), want(-):\n%s", diff)
			}
		})
	}
}
