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

package org

import (
	"encoding/json"
	"testing"
)

func TestRepoPermissionLevel(t *testing.T) {
	get := func(v RepoPermissionLevel) *RepoPermissionLevel {
		return &v
	}
	cases := []struct {
		input    string
		expected *RepoPermissionLevel
	}{
		{
			"admin",
			get(Admin),
		},
		{
			"write",
			get(Write),
		},
		{
			"read",
			get(Read),
		},
		{
			"none",
			get(None),
		},
		{
			"",
			nil,
		},
		{
			"unknown",
			nil,
		},
	}
	for _, tc := range cases {
		var actual RepoPermissionLevel
		err := json.Unmarshal([]byte("\""+tc.input+"\""), &actual)
		switch {
		case err == nil && tc.expected == nil:
			t.Errorf("%s: failed to receive an error", tc.input)
		case err != nil && tc.expected != nil:
			t.Errorf("%s: unexpected error: %v", tc.input, err)
		case err == nil && *tc.expected != actual:
			t.Errorf("%s: actual %v != expected %v", tc.input, tc.expected, actual)
		}
	}
}

func TestPrivacy(t *testing.T) {
	get := func(v Privacy) *Privacy {
		return &v
	}
	cases := []struct {
		input    string
		expected *Privacy
	}{
		{
			"secret",
			get(Secret),
		},
		{
			"closed",
			get(Closed),
		},
		{
			"",
			nil,
		},
		{
			"unknown",
			nil,
		},
	}
	for _, tc := range cases {
		var actual Privacy
		err := json.Unmarshal([]byte("\""+tc.input+"\""), &actual)
		switch {
		case err == nil && tc.expected == nil:
			t.Errorf("%s: failed to receive an error", tc.input)
		case err != nil && tc.expected != nil:
			t.Errorf("%s: unexpected error: %v", tc.input, err)
		case err == nil && *tc.expected != actual:
			t.Errorf("%s: actual %v != expected %v", tc.input, tc.expected, actual)
		}
	}
}
