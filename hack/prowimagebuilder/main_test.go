/*
Copyright 2022 The Kubernetes Authors.

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
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"k8s.io/test-infra/prow/flagutil"
)

func TestImageAllowed(t *testing.T) {
	tests := []struct {
		name   string
		images flagutil.Strings
		image  string
		want   bool
	}{
		{
			name:   "not-specified",
			images: flagutil.Strings{},
			image:  "whatever",
			want:   true,
		},
		{
			name:   "included",
			images: flagutil.NewStringsBeenSet("prow/cmd/awesome"),
			image:  "prow/cmd/awesome",
			want:   true,
		},
		{
			name:   "not-included",
			images: flagutil.NewStringsBeenSet("prow/cmd/awesome"),
			image:  "prow/cmd/otherawesome",
			want:   false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			o := options{
				images: tc.images,
			}
			got := o.imageAllowed(tc.image)
			if got != tc.want {
				t.Errorf("Unexpected. Want: %v, got: %v", tc.want, got)
			}
		})
	}
}

func TestRunCmdInDir(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		dir     string
		cmd     string
		args    []string
		wantOut string
		wantErr bool
	}{
		{
			name:    "echo",
			cmd:     "echo",
			args:    []string{"abc"},
			wantOut: "abc",
			wantErr: false,
		},
		{
			name:    "dir-not-exist",
			dir:     "dir-not-exist",
			cmd:     "echo",
			args:    []string{"abc"},
			wantOut: "",
			wantErr: true,
		},
		{
			name:    "dir-exist",
			dir:     dir,
			cmd:     "echo",
			args:    []string{"abc"},
			wantOut: "abc",
			wantErr: false,
		},
		{
			name:    "failed",
			cmd:     "echo-abc",
			args:    []string{"abc"},
			wantOut: "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, gotErr := runCmdInDir(tc.dir, nil, tc.cmd, tc.args...)
			if (gotErr != nil && !tc.wantErr) || (gotErr == nil && tc.wantErr) {
				t.Fatalf("Error mismatch. Want: %v, got: %v", tc.wantErr, gotErr)
			}
			if want, got := strings.TrimSpace(tc.wantOut), strings.TrimSpace(got); want != got {
				t.Fatalf("Output mismatch. Want: %q, got: %q", want, got)
			}
		})
	}
}

func TestRunCmd(t *testing.T) {
	gotOut, gotErr := runCmd(nil, "echo", "abc")
	if gotErr != nil {
		t.Fatalf("Should not error, got: %v", gotErr)
	}
	if want, got := "abc", strings.TrimSpace(gotOut); want != got {
		t.Fatalf("Output mismatch. Want: %q, got: %q", want, got)
	}
}

func TestLoadImageDefs(t *testing.T) {
	dir := t.TempDir()
	file := path.Join(dir, "temp.yaml")
	body := `images:
- dir: prow/cmd/admission
- dir: prow/cmd/clonerefs
  arch: all`

	wantDefs := []imageDef{
		{Dir: "prow/cmd/admission"},
		{Dir: "prow/cmd/clonerefs", Arch: "all"},
	}

	if err := os.WriteFile(file, []byte(body), 0644); err != nil {
		t.Fatalf("Failed write file: %v", err)
	}
	defs, err := loadImageDefs(file)
	if err != nil {
		t.Fatalf("Failed loading image defs: %v", err)
	}
	if diff := cmp.Diff(defs, wantDefs, cmpopts.IgnoreUnexported(imageDef{})); diff != "" {
		t.Fatalf("Output mismatch. Want(-), got(+):\n%s", diff)
	}
}

func TestAllTags(t *testing.T) {
	date, gitHash := "20220222", "a1b2c3d4"
	oldRunCmdInDirFunc := runCmdInDirFunc
	runCmdInDirFunc = func(dir string, additionalEnv []string, cmd string, args ...string) (string, error) {
		switch cmd {
		case "date":
			return date, nil
		case "git":
			return gitHash, nil
		default:
			return "", errors.New("not supported command")
		}
	}
	defer func() {
		runCmdInDirFunc = oldRunCmdInDirFunc
	}()

	tests := []struct {
		name string
		arch string
		want []string
	}{
		{
			name: "base",
			arch: "linux/amd64",
			want: []string{
				"latest",
				"latest-root",
				"20220222-a1b2c3d4",
				"ko-20220222-a1b2c3d4",
			},
		},
		{
			name: "other",
			arch: "linux/s390x",
			want: []string{
				"latest",
				"latest-root",
				"20220222-a1b2c3d4",
				"ko-20220222-a1b2c3d4",
				"latest-s390x",
				"latest-root-s390x",
				"20220222-a1b2c3d4-s390x",
				"ko-20220222-a1b2c3d4-s390x",
			},
		},
		{
			name: "all",
			arch: "all",
			want: []string{
				"latest",
				"latest-root",
				"20220222-a1b2c3d4",
				"ko-20220222-a1b2c3d4",
				"latest-arm64",
				"latest-root-arm64",
				"20220222-a1b2c3d4-arm64",
				"ko-20220222-a1b2c3d4-arm64",
				"latest-s390x",
				"latest-root-s390x",
				"20220222-a1b2c3d4-s390x",
				"ko-20220222-a1b2c3d4-s390x",
				"latest-ppc64le",
				"latest-root-ppc64le",
				"20220222-a1b2c3d4-ppc64le",
				"ko-20220222-a1b2c3d4-ppc64le",
			},
		},
		{
			// Not supported arches are caught in the invoker of this function,
			// not here.
			name: "not-supported",
			arch: "not/supported",
			want: []string{
				"latest",
				"latest-root",
				"20220222-a1b2c3d4",
				"ko-20220222-a1b2c3d4",
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, _ := allTags(tc.arch)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Tags mismatching. Want(-), got(+):\n%s", diff)
			}
		})
	}
}

func TestGitTag(t *testing.T) {
	tests := []struct {
		name    string
		date    string
		gitHash string
		cmdErr  error
		want    string
		wantErr bool
	}{
		{
			name:    "base",
			date:    "20220222",
			gitHash: "a1b2c3d4",
			want:    "20220222-a1b2c3d4",
		},
		{
			name:    "err",
			date:    "20220222",
			gitHash: "a1b2c3d4",
			cmdErr:  errors.New("error for test"),
			want:    "",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			oldRunCmdInDirFunc := runCmdInDirFunc
			runCmdInDirFunc = func(dir string, additionalEnv []string, cmd string, args ...string) (string, error) {
				switch cmd {
				case "date":
					return tc.date, tc.cmdErr
				case "git":
					return tc.gitHash, tc.cmdErr
				default:
					return "", errors.New("not supported command")
				}
			}
			defer func() {
				runCmdInDirFunc = oldRunCmdInDirFunc
			}()

			got, gotErr := gitTag()
			if (gotErr != nil && !tc.wantErr) || (gotErr == nil && tc.wantErr) {
				t.Fatalf("Error mismatch. Want: %v, got: %v", tc.wantErr, gotErr)
			}
			if want, got := strings.TrimSpace(tc.want), strings.TrimSpace(got); want != got {
				t.Fatalf("Output mismatch. Want: %q, got: %q", want, got)
			}
		})
	}
}

func TestBuildAndPush(t *testing.T) {
	date, gitHash := "20220222", "a1b2c3d4"

	tests := []struct {
		name         string
		id           imageDef
		koDockerRepo string
		push         bool
		want         []string
		wantErr      bool
	}{
		{
			name: "base",
			id: imageDef{
				Dir:  "prow/cmd/awesome",
				Arch: "linux/amd64",
			},
			koDockerRepo: "local.test",
			want: []string{
				"publish",
				"--tarball=_bin/awesome.tar",
				"--push=false",
				"--tags=latest",
				"--tags=latest-root",
				"--tags=20220222-a1b2c3d4",
				"--tags=ko-20220222-a1b2c3d4",
				"--base-import-paths",
				"--platform=linux/amd64",
				"./prow/cmd/awesome",
			},
		},
		{
			name: "push",
			id: imageDef{
				Dir:  "prow/cmd/awesome",
				Arch: "linux/amd64",
			},
			koDockerRepo: "local.test",
			push:         true,
			want: []string{
				"publish",
				"--push=true",
				"--tags=latest",
				"--tags=latest-root",
				"--tags=20220222-a1b2c3d4",
				"--tags=ko-20220222-a1b2c3d4",
				"--base-import-paths",
				"--platform=linux/amd64",
				"./prow/cmd/awesome",
			},
		},
		{
			name: "all",
			id: imageDef{
				Dir:  "prow/cmd/awesome",
				Arch: "all",
			},
			koDockerRepo: "local.test",
			push:         true,
			want: []string{
				"publish",
				"--push=true",
				"--tags=latest",
				"--tags=latest-root",
				"--tags=20220222-a1b2c3d4",
				"--tags=ko-20220222-a1b2c3d4",
				"--tags=latest-arm64",
				"--tags=latest-root-arm64",
				"--tags=20220222-a1b2c3d4-arm64",
				"--tags=ko-20220222-a1b2c3d4-arm64",
				"--tags=latest-s390x",
				"--tags=latest-root-s390x",
				"--tags=20220222-a1b2c3d4-s390x",
				"--tags=ko-20220222-a1b2c3d4-s390x",
				"--tags=latest-ppc64le",
				"--tags=latest-root-ppc64le",
				"--tags=20220222-a1b2c3d4-ppc64le",
				"--tags=ko-20220222-a1b2c3d4-ppc64le",
				"--base-import-paths",
				"--platform=all",
				"./prow/cmd/awesome",
			},
		},
		{
			name: "unsupported-arch",
			id: imageDef{
				Dir:  "prow/cmd/awesome",
				Arch: "not/supported",
			},
			koDockerRepo: "local.test",
			wantErr:      true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			var gotPublishArgs []string

			oldRunCmdInDirFunc := runCmdInDirFunc
			runCmdInDirFunc = func(dir string, additionalEnv []string, cmd string, args ...string) (string, error) {
				switch cmd {
				case "date":
					return date, nil
				case "git":
					return gitHash, nil
				case "_bin/ko":
					gotPublishArgs = args
					return fmt.Sprintf("cmd: %s, args: %v", cmd, args), nil
				default:
					return "", errors.New("not supported command")
				}
			}
			defer func() {
				runCmdInDirFunc = oldRunCmdInDirFunc
			}()

			err := buildAndPush(&tc.id, []string{tc.koDockerRepo}, tc.push)
			if !tc.wantErr && err != nil {
				t.Fatalf("Expect error but got nil")
			}
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Failed unexpected: %v", err)
				}
				return
			}

			if diff := cmp.Diff(tc.want, gotPublishArgs); diff != "" {
				t.Fatalf("Command mismatch. Want(-), got(+):\n%s", diff)
			}
		})
	}
}
