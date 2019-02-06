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
	"reflect"
	"sync"
	"testing"

	"k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/pod-utils/clone"
)

func TestRun(t *testing.T) {

	srcRoot, err := ioutil.TempDir("", "clonerefs_unittest")
	if err != nil {
		t.Fatalf("Error creating temp dir: %v.", err)
	}
	defer os.RemoveAll(srcRoot)

	type cloneRec struct {
		refs        kube.Refs
		root        string
		user, email string
		cookiePath  string
		env         []string
	}

	var recordedClones []cloneRec
	var lock sync.Mutex
	cloneFuncOld := cloneFunc
	cloneFunc = func(refs kube.Refs, root, user, email, cookiePath string, env []string) clone.Record {
		lock.Lock()
		defer lock.Unlock()
		recordedClones = append(recordedClones, cloneRec{
			refs:       refs,
			root:       root,
			user:       user,
			email:      email,
			cookiePath: cookiePath,
			env:        env,
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
				GitRefs: []kube.Refs{
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
				},
			},
			expectedClones: []cloneRec{
				{
					refs: kube.Refs{
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
				GitRefs: []kube.Refs{
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
					refs: kube.Refs{
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
					refs: kube.Refs{
						Org:       "kubernetes",
						Repo:      "release",
						BaseRef:   "master",
						PathAlias: "k8s.io/release",
					},
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
