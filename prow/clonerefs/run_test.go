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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	v1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/pod-utils/clone"
)

func TestRun(t *testing.T) {
	srcRoot := t.TempDir()

	oauthTokenDir := t.TempDir()
	oauthTokenFilePath := filepath.Join(oauthTokenDir, "oauth-token")
	oauthTokenValue := []byte("12345678")
	if err := os.WriteFile(oauthTokenFilePath, oauthTokenValue, 0644); err != nil {
		t.Fatalf("Error while create oauth token file: %v", err)
	}

	githubAppDir := t.TempDir()
	githubAppPrivateKeyFilePath := filepath.Join(githubAppDir, "private-key.pem")
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		t.Fatalf("Error while create github app private key file: %v", err)
	}
	githubAppPrivateKeyValue := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	if err := os.WriteFile(githubAppPrivateKeyFilePath, githubAppPrivateKeyValue, 0644); err != nil {
		t.Fatalf("Error while create github app private key file: %v", err)
	}

	githubAppOrg := "kubernetes"
	githubAppToken := "github-app-token"
	mockGitHubAppServer := httptest.NewServer(mockGitHubAppHandler(githubAppOrg, githubAppToken))
	defer mockGitHubAppServer.Close()

	type cloneRec struct {
		refs        prowapi.Refs
		root        string
		user, email string
		cookiePath  string
		env         []string
		authUser    string
		authToken   string
		authError   error
	}

	var recordedClones []cloneRec
	var lock sync.Mutex
	cloneFuncOld := cloneFunc
	cloneFunc = func(refs prowapi.Refs, root, user, email, cookiePath string, env []string, userGenerator github.UserGenerator, tokenGenerator github.TokenGenerator) clone.Record {
		lock.Lock()
		defer lock.Unlock()
		var (
			authUser  string
			authToken string
			authError error
		)
		if userGenerator != nil {
			user, err := userGenerator()
			if err != nil {
				authError = err
			}
			authUser = user
		}
		if tokenGenerator != nil {
			token, err := tokenGenerator(refs.Org)
			if err != nil {
				authError = err
			}
			authToken = token
		}
		recordedClones = append(recordedClones, cloneRec{
			refs:       refs,
			root:       root,
			user:       user,
			email:      email,
			cookiePath: cookiePath,
			env:        env,
			authUser:   authUser,
			authToken:  authToken,
			authError:  authError,
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
					authToken:  "12345678",
				},
			},
		},
		{
			name: "single PR clone with GitHub App",
			opts: Options{
				GitHubAPIEndpoints: []string{
					mockGitHubAppServer.URL,
				},
				GitHubAppID:             "123456",
				GitHubAppPrivateKeyFile: githubAppPrivateKeyFilePath,
				SrcRoot:                 srcRoot,
				Log:                     path.Join(srcRoot, "log.txt"),
				GitUserName:             "me",
				GitUserEmail:            "me@domain.com",
				CookiePath:              "cookies/path",
				GitRefs: []prowapi.Refs{
					{
						Org:       githubAppOrg,
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
					authUser:   "x-access-token",
					authToken:  githubAppToken,
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

func mockGitHubAppHandler(org, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/app":
			json.NewEncoder(w).Encode(github.App{
				Slug: "slug",
			})
		case "/app/installations":
			json.NewEncoder(w).Encode([]github.AppInstallation{
				{
					ID: 1,
					Account: github.User{
						Login: org,
					},
				},
			})
		case "/app/installations/1/access_tokens":
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(&github.AppInstallationToken{
				Token:     token,
				ExpiresAt: time.Now().Add(time.Minute),
			})
		default:
			fmt.Println(r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}
