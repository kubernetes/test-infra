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

package main

import (
	"context"
	"go/build"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	coreapi "k8s.io/api/core/v1"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
)

func TestPathAlias(t *testing.T) {
	cases := []struct {
		name string
		pa   string
		org  string
		repo string
		want string
	}{
		{
			name: "with path alias",
			pa:   "pa",
			org:  "org1",
			repo: "repo1",
			want: "pa",
		},
		{
			name: "without path alias",
			pa:   "",
			org:  "org1",
			repo: "repo1",
			want: "github.com/org1/repo1",
		},
		{
			name: "org name starts with http://",
			pa:   "",
			org:  "http://org1",
			repo: "repo1",
			want: "http://org1/repo1",
		},
		{
			name: "org name starts with https://",
			pa:   "",
			org:  "https://org1",
			repo: "repo1",
			want: "https://org1/repo1",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := prowapi.Refs{
				PathAlias: tc.pa,
				Org:       tc.org,
				Repo:      tc.repo,
			}
			if got := pathAlias(r); got != tc.want {
				t.Fatalf("Failed getting path alias. Want: %s, got: %s", tc.want, got)
			}
		})
	}
}

func TestReadRepo(t *testing.T) {
	dir, err := ioutil.TempDir("", "read-repo")
	if err != nil {
		t.Fatalf("Cannot create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	cases := []struct {
		name      string
		goal      string
		wd        string
		gopath    string
		dirs      []string
		userInput string
		expected  string
		err       bool
	}{
		{
			name:     "find from local",
			goal:     "k8s.io/test-infra2",
			wd:       path.Join(dir, "find_from_local", "go/src/k8s.io/test-infra2"),
			expected: path.Join(dir, "find_from_local", "go/src/k8s.io/test-infra2"),
		},
		{
			name:     "find from local fallback",
			goal:     "k8s.io/test-infra2",
			wd:       path.Join(dir, "find_from_local_fallback", "go/src/test-infra2"),
			expected: path.Join(dir, "find_from_local_fallback", "go/src/test-infra2"),
		},
		{
			name:   "find from explicit gopath",
			goal:   "k8s.io/test-infra2",
			gopath: path.Join(dir, "find_from_explicit_gopath"),
			wd:     path.Join(dir, "find_from_explicit_gopath_random", "random"),
			dirs: []string{
				path.Join(dir, "find_from_explicit_gopath", "src", "k8s.io/test-infra2"),
				path.Join(dir, "find_from_explicit_gopath_random", "src", "test-infra2"),
			},
			expected: path.Join(dir, "find_from_explicit_gopath", "src", "k8s.io/test-infra2"),
		},
		{
			name:   "prefer gopath",
			goal:   "k8s.io/test-infra2",
			gopath: path.Join(dir, "prefer_gopath"),
			wd:     path.Join(dir, "prefer_gopath_random", "random"),
			dirs: []string{
				path.Join(dir, "prefer_gopath", "src", "k8s.io/test-infra2"),
				path.Join(dir, "prefer_gopath", "src", "test-infra2"),
			},
			expected: path.Join(dir, "prefer_gopath", "src", "k8s.io/test-infra2"),
		},
		{
			name:   "not exist",
			goal:   "k8s.io/test-infra2",
			gopath: path.Join(dir, "not_exist", "random1"),
			wd:     path.Join(dir, "not_exist", "random2"),
			err:    true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.dirs = append(tc.dirs, tc.wd)
			for _, d := range tc.dirs {
				if err := os.MkdirAll(d, 0755); err != nil {
					t.Fatalf("Cannot create subdir %q: %v", d, err)
				}
			}

			// build.Default was loaded while imported, override it directly.
			oldGopath := build.Default.GOPATH
			defer func() {
				build.Default.GOPATH = oldGopath
			}()
			build.Default.GOPATH = tc.gopath

			// Trick the system to think it's running in bazel and wd is tc.wd.
			oldPwd := os.Getenv("BUILD_WORKING_DIRECTORY")
			defer os.Setenv("BUILD_WORKING_DIRECTORY", oldPwd)
			os.Setenv("BUILD_WORKING_DIRECTORY", tc.wd)

			actual, err := readRepo(tc.goal, func(path, def string) (string, error) {
				return tc.userInput, nil
			})
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("Unexpected error: %v", err)
				}
			case tc.err:
				t.Error("Failed to get an error")
			case actual != tc.expected:
				t.Errorf("Actual %q != expected %q", actual, tc.expected)
			}
		})
	}
}

func TestFindRepoFromLocal(t *testing.T) {
	cases := []struct {
		name     string
		goal     string
		wd       string
		dirs     []string
		expected string
		err      bool
	}{
		{
			name:     "match full repo",
			goal:     "k8s.io/test-infra",
			wd:       "go/src/k8s.io/test-infra",
			expected: "go/src/k8s.io/test-infra",
		},
		{
			name: "repo not found",
			goal: "k8s.io/test-infra",
			wd:   "random",
			dirs: []string{"k8s.io/repo-infra", "github.com/fejta/test-infra"},
			err:  true,
		},
		{
			name:     "convert github to k8s vanity",
			goal:     "github.com/kubernetes/test-infra",
			wd:       "go/src/k8s.io/test-infra",
			expected: "go/src/k8s.io/test-infra",
		},
		{
			name:     "match sibling base",
			goal:     "k8s.io/repo-infra",
			wd:       "src/test-infra",
			dirs:     []string{"src/repo-infra"},
			expected: "src/repo-infra",
		},
		{
			name:     "match full sibling",
			goal:     "k8s.io/repo-infra",
			wd:       "go/src/k8s.io/test-infra",
			dirs:     []string{"go/src/k8s.io/repo-infra"},
			expected: "go/src/k8s.io/repo-infra",
		},
		{
			name:     "match just repo",
			goal:     "k8s.io/test-infra",
			wd:       "src/test-infra",
			expected: "src/test-infra",
		},
		{
			name:     "match base of repo",
			goal:     "k8s.io/test-infra",
			wd:       "src/test-infra/prow/cmd/mkpj",
			expected: "src/test-infra",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "find-repo-"+tc.name)
			if err != nil {
				t.Fatalf("Cannot create temp dir: %v", err)
			}
			defer os.RemoveAll(dir)

			tc.dirs = append(tc.dirs, tc.wd)
			for _, d := range tc.dirs {
				full := filepath.Join(dir, d)
				if err := os.MkdirAll(full, 0755); err != nil {
					t.Fatalf("Cannot create subdir %q: %v", full, err)
				}
			}

			wd := filepath.Join(dir, tc.wd)
			expected := filepath.Join(dir, tc.expected)

			actual, err := findRepoFromLocal(wd, tc.goal)
			switch {
			case err != nil:
				if !tc.err {
					t.Errorf("Unexpected error: %v", err)
				}
			case tc.err:
				t.Error("Failed to get an error")
			case actual != expected:
				t.Errorf("Actual %q != expected %q", actual, expected)
			}
		})
	}
}

func TestResolveVolumeMounts(t *testing.T) {
	cvm := []coreapi.VolumeMount{
		{
			Name:      "volume1",
			MountPath: "/whatever/mountpath1",
		},
		{
			Name:      "volume2",
			MountPath: "/whatever/mountpath2",
		},
	}
	pv := []coreapi.Volume{
		{
			Name: "volume1",
			VolumeSource: coreapi.VolumeSource{
				HostPath: &coreapi.HostPathVolumeSource{
					Path: "/whatever/hostpath1",
				},
			},
		},
		{
			Name: "volume2",
			VolumeSource: coreapi.VolumeSource{
				HostPath: &coreapi.HostPathVolumeSource{
					Path: "/whatever/hostpath2",
				},
			},
		},
	}
	fakeReadMount := func(ctx context.Context, mount coreapi.VolumeMount) (string, error) {
		return strings.Replace(mount.MountPath, "mountpath", "hostpath", -1), nil
	}

	cases := []struct {
		name                  string
		containerVolumeMounts []coreapi.VolumeMount
		podVolumes            []coreapi.Volume
		skippedVolumeMounts   []string
		extraVolumeMounts     map[string]string
		expectErr             bool
		expected              map[string]string
	}{
		{
			name:     "no volume mounts",
			expected: map[string]string{},
		},
		{
			name:                  "only container volume mounts",
			containerVolumeMounts: cvm,
			podVolumes:            pv,
			expected: map[string]string{
				"/whatever/mountpath1": "/whatever/hostpath1",
				"/whatever/mountpath2": "/whatever/hostpath2",
			},
		},
		{
			name:                  "no corresponding Pod volumes",
			containerVolumeMounts: cvm,
			podVolumes:            nil,
			expectErr:             true,
		},
		{
			name: "empty dir volume mount",
			containerVolumeMounts: []coreapi.VolumeMount{
				{
					Name:      "empty-volume",
					MountPath: "/whatever/mountpath1",
				},
			},
			podVolumes: []coreapi.Volume{
				{
					Name: "empty-volume",
					VolumeSource: coreapi.VolumeSource{
						EmptyDir: &coreapi.EmptyDirVolumeSource{},
					},
				},
			},
			expected: map[string]string{
				"/whatever/mountpath1": "",
			},
		},
		{
			name: "readonly volume mount",
			containerVolumeMounts: []coreapi.VolumeMount{
				{
					Name:      "readonly-volume",
					MountPath: "/whatever/mountpath1",
					ReadOnly:  true,
				},
			},
			podVolumes: []coreapi.Volume{
				{
					Name: "readonly-volume",
					VolumeSource: coreapi.VolumeSource{
						HostPath: &coreapi.HostPathVolumeSource{
							Path: "/whatever/hostpath1",
						},
					},
				},
			},
			expected: map[string]string{
				"/whatever/mountpath1:ro": "/whatever/hostpath1",
			},
		},
		{
			name:                  "skip some volume mounts",
			containerVolumeMounts: cvm,
			podVolumes:            pv,
			skippedVolumeMounts:   []string{"volume1"},
			expected: map[string]string{
				"/whatever/mountpath2": "/whatever/hostpath2",
			},
		},
		{
			name:                  "add extra volume mounts",
			containerVolumeMounts: cvm,
			podVolumes:            pv,
			extraVolumeMounts: map[string]string{
				"/whatever/mountpath3": "/whatever/hostpath3",
			},
			expected: map[string]string{
				"/whatever/mountpath1": "/whatever/hostpath1",
				"/whatever/mountpath2": "/whatever/hostpath2",
				"/whatever/mountpath3": "/whatever/hostpath3",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := options{
				skippedVolumesMounts: tc.skippedVolumeMounts,
				extraVolumesMounts:   tc.extraVolumeMounts,
			}
			container := coreapi.Container{
				VolumeMounts: tc.containerVolumeMounts,
			}
			pj := prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					PodSpec: &coreapi.PodSpec{
						Volumes: tc.podVolumes,
					},
				},
			}
			got, err := opts.resolveVolumeMounts(context.Background(), pj, container, fakeReadMount)

			if err != nil {
				if !tc.expectErr {
					t.Errorf("Unexpected error: %v", err)
				}
			} else if tc.expectErr {
				t.Error("Failed to get an error")
			}
			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("resolveEnvVars returns wrong result (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveEnvVars(t *testing.T) {
	cases := []struct {
		name             string
		containerEnvVars []coreapi.EnvVar
		skippedEnvVars   []string
		extraEnvVars     map[string]string
		expected         map[string]string
	}{
		{
			name:     "no env vars",
			expected: map[string]string{},
		},
		{
			name: "only container env vars",
			containerEnvVars: []coreapi.EnvVar{
				{
					Name:  "env_key1",
					Value: "env_val1",
				},
				{
					Name:  "env_key2",
					Value: "env_val2",
				},
			},
			expected: map[string]string{
				"env_key1": "env_val1",
				"env_key2": "env_val2",
			},
		},
		{
			name: "skip some env vars",
			containerEnvVars: []coreapi.EnvVar{
				{
					Name:  "env_key1",
					Value: "env_val1",
				},
				{
					Name:  "env_key2",
					Value: "env_val2",
				},
			},
			skippedEnvVars: []string{"env_key1"},
			expected: map[string]string{
				"env_key2": "env_val2",
			},
		},
		{
			name: "add extra env vars",
			containerEnvVars: []coreapi.EnvVar{
				{
					Name:  "env_key1",
					Value: "env_val1",
				},
			},
			extraEnvVars: map[string]string{
				"env_key2": "env_val2",
			},
			expected: map[string]string{
				"env_key1": "env_val1",
				"env_key2": "env_val2",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := options{
				skippedEnvVars: tc.skippedEnvVars,
				extraEnvVars:   tc.extraEnvVars,
			}
			container := coreapi.Container{
				Env: tc.containerEnvVars,
			}
			got := opts.resolveEnvVars(container)
			if diff := cmp.Diff(tc.expected, got); diff != "" {
				t.Errorf("resolveEnvVars returns wrong result (-want +got):\n%s", diff)
			}
		})
	}
}
