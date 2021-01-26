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

package clonerefs

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/pod-utils/clone"
)

func TestRun(t *testing.T) {

	srcRoot, err := ioutil.TempDir("", "clonerefs_unittest")
	if err != nil {
		t.Fatalf("Error while creating temp dir: %v.", err)
	}
	defer os.RemoveAll(srcRoot)

	oauthTokenDir, err := ioutil.TempDir("", "oauth")
	if err != nil {
		t.Fatalf("Error while creating oauth token dir: %v.", err)
	}
	defer os.RemoveAll(oauthTokenDir)

	oauthTokenFilePath := filepath.Join(oauthTokenDir, "oauth-token")
	oauthTokenValue := []byte("12345678")
	if err := ioutil.WriteFile(oauthTokenFilePath, oauthTokenValue, 0644); err != nil {
		t.Fatalf("Error while create oauth token file: %v", err)
	}

	type cloneRec struct {
		refs        prowapi.Refs
		root        string
		user, email string
		cookiePath  string
		env         []string
		oauthToken  string
	}

	var recordedClones []cloneRec
	var lock sync.Mutex
	cloneFuncOld := cloneFunc
	cloneFunc = func(refs prowapi.Refs, root, user, email, cookiePath string, env []string, oauthToken string) clone.Record {
		lock.Lock()
		defer lock.Unlock()
		recordedClones = append(recordedClones, cloneRec{
			refs:       refs,
			root:       root,
			user:       user,
			email:      email,
			cookiePath: cookiePath,
			env:        env,
			oauthToken: oauthToken,
		})
		return clone.Record{}
	}
	defer func() { cloneFunc = cloneFuncOld }()

	testcases := []struct {
		name           string
		opts           Options
		expectedClones []cloneRec
	}{
		{
			name: "single PR clone",
			opts: Options{
				SrcRoot:      srcRoot,
				Log:          path.Join(srcRoot, "log.txt"),
				GitUserName:  "me",
				GitUserEmail: "me@domain.com",
				CookiePath:   "cookies/path",
				GitRefs: []prowapi.Refs{
					{
						Org:       "kubernetes",
						Repo:      "test-infra",
						BaseRef:   "master",
						PathAlias: "k8s.io/test-infra",
						Pulls: []v1.Pull{
							{
								Number: 5,
								SHA:    "FEEDDAD",
							},
						},
						SkipSubmodules: true,
					},
				},
			},
			expectedClones: []cloneRec{
				{
					refs: prowapi.Refs{
						Org:       "kubernetes",
						Repo:      "test-infra",
						BaseRef:   "master",
						PathAlias: "k8s.io/test-infra",
						Pulls: []v1.Pull{
							{
								Number: 5,
								SHA:    "FEEDDAD",
							},
						},
						SkipSubmodules: true,
					},
					root:       srcRoot,
					user:       "me",
					email:      "me@domain.com",
					cookiePath: "cookies/path",
				},
			},
		},
		{
			name: "multi repo clone",
			opts: Options{
				Log: path.Join(srcRoot, "log.txt"),
				GitRefs: []prowapi.Refs{
					{
						Org:       "kubernetes",
						Repo:      "test-infra",
						BaseRef:   "master",
						PathAlias: "k8s.io/test-infra",
						Pulls: []v1.Pull{
							{
								Number: 5,
								SHA:    "FEEDDAD",
							},
						},
					},
					{
						Org:       "kubernetes",
						Repo:      "release",
						BaseRef:   "master",
						PathAlias: "k8s.io/release",
					},
				},
			},
			expectedClones: []cloneRec{
				{
					refs: prowapi.Refs{
						Org:       "kubernetes",
						Repo:      "test-infra",
						BaseRef:   "master",
						PathAlias: "k8s.io/test-infra",
						Pulls: []v1.Pull{
							{
								Number: 5,
								SHA:    "FEEDDAD",
							},
						},
					},
				},
				{
					refs: prowapi.Refs{
						Org:       "kubernetes",
						Repo:      "release",
						BaseRef:   "master",
						PathAlias: "k8s.io/release",
					},
				},
			},
		},
		{
			name: "single PR clone with oauth token",
			opts: Options{
				OauthTokenFile: oauthTokenFilePath,
				SrcRoot:        srcRoot,
				Log:            path.Join(srcRoot, "log.txt"),
				GitUserName:    "me",
				GitUserEmail:   "me@domain.com",
				CookiePath:     "cookies/path",
				GitRefs: []prowapi.Refs{
					{
						Org:       "kubernetes",
						Repo:      "test-infra",
						BaseRef:   "master",
						PathAlias: "k8s.io/test-infra",
						Pulls: []v1.Pull{
							{
								Number: 5,
								SHA:    "FEEDDAD",
							},
						},
						SkipSubmodules: true,
					},
				},
			},
			expectedClones: []cloneRec{
				{
					refs: prowapi.Refs{
						Org:       "kubernetes",
						Repo:      "test-infra",
						BaseRef:   "master",
						PathAlias: "k8s.io/test-infra",
						Pulls: []v1.Pull{
							{
								Number: 5,
								SHA:    "FEEDDAD",
							},
						},
						SkipSubmodules: true,
					},
					root:       srcRoot,
					user:       "me",
					email:      "me@domain.com",
					cookiePath: "cookies/path",
					oauthToken: "12345678",
				},
			},
		},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() { recordedClones = nil }()
			os.RemoveAll(srcRoot)
			os.MkdirAll(srcRoot, os.ModePerm)

			if err := tc.opts.Run(); err != nil {
				t.Fatalf("Unexpected error: %v.", err)
			}

			// Check for set equality (ignore ordering)
			for _, rec := range recordedClones {
				found := false
				var exp cloneRec
				for _, exp = range tc.expectedClones {
					if reflect.DeepEqual(rec, exp) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("recordedClones %#v is missing expected clone %#v", recordedClones, exp)
				}
			}
			if rec, exp := len(recordedClones), len(tc.expectedClones); rec != exp {
				t.Errorf("recordedClones has length %d and expectedClones has length %d", rec, exp)
			}

		})
	}
}

func TestNeedsGlobalCookiePath(t *testing.T) {
	cases := []struct {
		name       string
		cookieFile string
		refs       []prowapi.Refs
		expected   string
	}{
		{
			name: "basically works",
		},
		{
			name: "return empty when no cookieFile",
			refs: []prowapi.Refs{
				{},
			},
		},
		{
			name:       "return empty when no refs",
			cookieFile: "foo",
		},
		{
			name:       "return empty when all refs skip submodules",
			cookieFile: "foo",
			refs: []prowapi.Refs{
				{SkipSubmodules: true},
				{SkipSubmodules: true},
			},
		},
		{
			name:       "return cookieFile when all refs use submodules",
			cookieFile: "foo",
			refs: []prowapi.Refs{
				{},
				{},
			},
			expected: "foo",
		},
		{
			name:       "return cookieFile when any refs uses submodules",
			cookieFile: "foo",
			refs: []prowapi.Refs{
				{SkipSubmodules: true},
				{},
			},
			expected: "foo",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if actual := needsGlobalCookiePath(tc.cookieFile, tc.refs...); actual != tc.expected {
				t.Errorf("needsGlobalCookiePath(%q,%v) got %q, want %q", tc.cookieFile, tc.refs, actual, tc.expected)
			}
		})
	}
}
