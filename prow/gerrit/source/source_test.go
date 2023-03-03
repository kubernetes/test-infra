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

// Package source contains functions that help with Gerrit source control
// specific logics.
package source

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestCloneURIFromOrgRepo(t *testing.T) {
	tests := []struct {
		name string
		org  string
		repo string
		want string
	}{
		{
			name: "base",
			org:  "foo.baz",
			repo: "bar",
			want: "https://foo.baz/bar",
		},
		{
			name: "with-https",
			org:  "https://foo.baz",
			repo: "bar",
			want: "https://foo.baz/bar",
		},
		{
			name: "with-http",
			org:  "http://foo.baz",
			repo: "bar",
			want: "http://foo.baz/bar",
		},
		{
			name: "org-extra-slash",
			org:  "https://foo.baz/",
			repo: "bar",
			want: "https://foo.baz/bar",
		},
		{
			name: "repo-extra-slash",
			org:  "https://foo.baz",
			repo: "/bar/",
			want: "https://foo.baz/bar",
		},
	}

	for _, tc := range tests {
		tc := tc
		if got, want := CloneURIFromOrgRepo(tc.org, tc.repo), tc.want; got != want {
			t.Errorf("CloneURI mismatch. Want: '%s', got: '%s'", want, got)
		}
	}
}

func TestNormalizeOrg(t *testing.T) {
	tests := []struct {
		name string
		org  string
		want string
	}{
		{
			name: "base",
			org:  "foo.baz",
			want: "https://foo.baz",
		},
		{
			name: "with-https",
			org:  "https://foo.baz",
			want: "https://foo.baz",
		},
		{
			name: "with-http",
			org:  "http://foo.baz",
			want: "http://foo.baz",
		},
		{
			name: "org-extra-slash",
			org:  "https://foo.baz/",
			want: "https://foo.baz",
		},
	}

	for _, tc := range tests {
		tc := tc
		if got, want := NormalizeOrg(tc.org), tc.want; got != want {
			t.Errorf("CloneURI mismatch. Want: '%s', got: '%s'", want, got)
		}
	}
}

func TestNormalizeCloneURI(t *testing.T) {
	tests := []struct {
		name     string
		cloneURI string
		want     string
	}{
		{
			name:     "base",
			cloneURI: "foo.baz",
			want:     "https://foo.baz",
		},
		{
			name:     "with-https",
			cloneURI: "https://foo.baz",
			want:     "https://foo.baz",
		},
		{
			name:     "with-http",
			cloneURI: "http://foo.baz",
			want:     "http://foo.baz",
		},
		{
			name:     "org-extra-slash",
			cloneURI: "https://foo.baz/",
			want:     "https://foo.baz",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if got, want := NormalizeCloneURI(tc.cloneURI), tc.want; got != want {
				t.Errorf("CloneURI mismatch. Want: '%s', got: '%s'", want, got)
			}
		})
	}
}

func TestOrgRepoFromCloneURI(t *testing.T) {
	type orgRepo struct {
		Org, Repo string
	}
	tests := []struct {
		name     string
		cloneURI string
		want     orgRepo
		wantErr  error
	}{
		{
			name:     "base",
			cloneURI: "foo.baz/bar",
			want:     orgRepo{"https://foo.baz", "bar"},
		},
		{
			name:     "with-https",
			cloneURI: "https://foo.baz/bar",
			want:     orgRepo{"https://foo.baz", "bar"},
		},
		{
			name:     "with-http",
			cloneURI: "http://foo.baz/bar",
			want:     orgRepo{"http://foo.baz", "bar"},
		},
		{
			name:     "org-extra-slash",
			cloneURI: "https://foo.baz/bar//",
			want:     orgRepo{"https://foo.baz", "bar"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			gotOrg, gotRepo, gotErr := OrgRepoFromCloneURI(tc.cloneURI)
			if tc.wantErr != nil {
				if gotErr != tc.wantErr {
					t.Fatalf("Error mismatch. Want: %v, got: %v", tc.wantErr, gotErr)
				}
				return
			}
			if diff := cmp.Diff(tc.want, orgRepo{gotOrg, gotRepo}); diff != "" {
				t.Errorf("CloneURI mismatch. Want(-), got(+):\n%s", diff)
			}
		})
	}
}

func TestCodeRootURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{
			name: "base",
			in:   "https://foo-review.googlesource.com",
			want: "https://foo.googlesource.com",
		},
		{
			name: "with-repo",
			in:   "https://foo-review.googlesource.com/bar",
			want: "https://foo.googlesource.com/bar",
		},
		{
			name:    "invalid",
			in:      "https://foo.googlesource.com",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, gotErr := CodeRootURL(tc.in)
			if tc.wantErr {
				if gotErr == nil {
					t.Fatal("Want error, got nil")
				}
				return
			}
			if gotErr != nil {
				t.Fatalf("Want no error, got: %v", gotErr)
			}
			if want, got := tc.want, got; want != got {
				t.Fatalf("Want: %s, got: %s", want, got)
			}
		})
	}
}
