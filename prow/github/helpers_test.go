/*
Copyright 2019 The Kubernetes Authors.

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

package github

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
			"maintain",
			get(Maintain),
		},
		{
			"write",
			get(Write),
		},
		{
			"triage",
			get(Triage),
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

func TestLevelFromPermissions(t *testing.T) {
	var testCases = []struct {
		permissions RepoPermissions
		level       RepoPermissionLevel
	}{
		{
			permissions: RepoPermissions{},
			level:       None,
		},
		{
			permissions: RepoPermissions{Pull: true},
			level:       Read,
		},
		{
			permissions: RepoPermissions{Pull: true, Triage: true},
			level:       Triage,
		},
		{
			permissions: RepoPermissions{Pull: true, Triage: true, Push: true},
			level:       Write,
		},
		{
			permissions: RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true},
			level:       Maintain,
		},
		{
			permissions: RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true, Admin: true},
			level:       Admin,
		},
	}

	for _, testCase := range testCases {
		if actual, expected := LevelFromPermissions(testCase.permissions), testCase.level; actual != expected {
			t.Errorf("got incorrect level from permissions, expected %v but got %v", expected, actual)
		}
	}
}

func TestPermissionsFromTeamPermission(t *testing.T) {
	var testCases = []struct {
		level       TeamPermission
		permissions RepoPermissions
	}{
		{
			level:       TeamPermission("foobar"),
			permissions: RepoPermissions{},
		},
		{
			level:       RepoPull,
			permissions: RepoPermissions{Pull: true},
		},
		{
			level:       RepoTriage,
			permissions: RepoPermissions{Pull: true, Triage: true},
		},
		{
			level:       RepoPush,
			permissions: RepoPermissions{Pull: true, Triage: true, Push: true},
		},
		{
			level:       RepoMaintain,
			permissions: RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true},
		},
		{
			level:       RepoAdmin,
			permissions: RepoPermissions{Pull: true, Triage: true, Push: true, Maintain: true, Admin: true},
		},
	}

	for _, testCase := range testCases {
		if actual, expected := PermissionsFromTeamPermission(testCase.level), testCase.permissions; actual != expected {
			t.Errorf("got incorrect permissions from team permissions, expected %v but got %v", expected, actual)
		}
	}
}
