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

package git

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestRefreshRepoAuth(t *testing.T) {
	g, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		repo       *Repo
		wantPass   string
		wantRemote string
		wantErr    bool
	}{
		{
			name: "base",
			repo: &Repo{
				base: "git",
				user: "foo",
				pass: "preset",
				org:  "valid-org",
				repo: "repo",
			},
			wantPass:   "valid-token",
			wantRemote: "https://foo:valid-token@/valid-org/repo",
			wantErr:    false,
		},
		{
			name: "empty-user",
			repo: &Repo{
				base: "git",
				pass: "preset",
				org:  "valid-org",
				repo: "repo",
			},
			wantPass:   "valid-token",
			wantRemote: "git/valid-org/repo",
			wantErr:    false,
		},
		{
			name: "token-still-fresh",
			repo: &Repo{
				base: "git",
				user: "foo",
				pass: "valid-token",
				org:  "valid-org",
				repo: "repo",
			},
			wantPass:   "valid-token",
			wantRemote: "preset/remote",
			wantErr:    false,
		},
		{
			name: "token-fetch-failed",
			repo: &Repo{
				base: "git",
				user: "foo",
				pass: "preset",
				org:  "random-org",
				repo: "repo",
			},
			wantPass:   "preset",
			wantRemote: "preset/remote",
			wantErr:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.repo
			r.logger = logrus.WithContext(context.Background())
			r.git = g
			r.dir = t.TempDir()
			r.tokenGenerator = func(org string) (string, error) {
				switch org {
				case "valid-org":
					return "valid-token", nil
				case "invalid-org":
					return "invalid-token", nil
				default:
					return "", errors.New("error")
				}
			}

			// Prepare for the workspace so that git command won't fail.
			b, err := r.gitCommand("init").CombinedOutput()
			if err != nil {
				t.Fatalf("Failed init: %v. output: %s", err, string(b))
			}
			if b, err = r.gitCommand("remote", "add", "origin", "preset/remote").CombinedOutput(); err != nil {
				t.Fatalf("Failed set origin: %v. output: %s", err, string(b))
			}

			if gotErr := r.refreshRepoAuth(); (gotErr != nil && !tc.wantErr) || (gotErr == nil && tc.wantErr) {
				t.Fatalf("Error mismatch. Want: %v, got: %v", tc.wantErr, gotErr)
			}

			if wantPass, gotPass := tc.wantPass, r.pass; wantPass != gotPass {
				t.Fatalf("Wrong token. Want: %q, got: %q", wantPass, gotPass)
			}

			b, err = r.gitCommand("remote", "get-url", "origin").CombinedOutput()
			if err != nil {
				t.Fatalf("Failed git config: %v. output: %s", err, string(b))
			}
			if wantRemote, gotRemote := tc.wantRemote, strings.TrimSpace(string(b)); wantRemote != gotRemote {
				t.Fatalf("Wrong remote. Want: %q, got: %q", wantRemote, gotRemote)
			}
		})
	}
}
