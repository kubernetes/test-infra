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

package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/kube"
)

type fakeBucket struct {
	name    string
	objects map[string]string
}

func (bucket fakeBucket) getName() string {
	return bucket.name
}

func (bucket fakeBucket) listSubDirs(prefix string) ([]string, error) {
	dirs := sets.String{}
	for k := range bucket.objects {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		suffix := strings.TrimPrefix(k, prefix)
		dir := strings.Split(suffix, "/")[0]
		dirs.Insert(dir)
	}
	return dirs.List(), nil
}

func (bucket fakeBucket) listAll(prefix string) ([]string, error) {
	keys := []string{}
	for k := range bucket.objects {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (bucket fakeBucket) readObject(key string) ([]byte, error) {
	if obj, ok := bucket.objects[key]; ok {
		return []byte(obj), nil
	}
	return []byte{}, fmt.Errorf("object %s not found", key)
}

func TestUpdateCommitData(t *testing.T) {
	cases := []struct {
		name      string
		hash      string
		buildTime time.Time
		width     int
		before    map[string]*commitData
		after     map[string]*commitData
	}{
		{
			name:      "new commit",
			hash:      "d0c3cd182cffb3e722b14322fd1ca854a8bf62b0",
			width:     1,
			buildTime: time.Unix(1543534799, 0),
			before:    make(map[string]*commitData),
			after: map[string]*commitData{
				"d0c3cd182cffb3e722b14322fd1ca854a8bf62b0": {
					HashPrefix: "d0c3cd1",
					MaxWidth:   1,
					Link:       "https://github.com/kubernetes/test-infra/commit/d0c3cd182cffb3e722b14322fd1ca854a8bf62b0",
					latest:     time.Unix(1543534799, 0),
				},
			},
		},
		{
			name:      "update existing commit",
			hash:      "d0c3cd182cffb3e722b14322fd1ca854a8bf62b0",
			width:     3,
			buildTime: time.Unix(320630400, 0),
			before: map[string]*commitData{
				"d0c3cd182cffb3e722b14322fd1ca854a8bf62b0": {
					HashPrefix: "d0c3cd1",
					MaxWidth:   5,
					Link:       "https://github.com/kubernetes/test-infra/commit/d0c3cd182cffb3e722b14322fd1ca854a8bf62b0",
					latest:     time.Unix(0, 0),
				},
			},
			after: map[string]*commitData{
				"d0c3cd182cffb3e722b14322fd1ca854a8bf62b0": {
					HashPrefix: "d0c3cd1",
					MaxWidth:   5,
					Link:       "https://github.com/kubernetes/test-infra/commit/d0c3cd182cffb3e722b14322fd1ca854a8bf62b0",
					latest:     time.Unix(320630400, 0),
				},
			},
		},
		{
			name:   "unknown commit has no link",
			hash:   "unknown",
			width:  1,
			before: make(map[string]*commitData),
			after: map[string]*commitData{
				"unknown": {
					HashPrefix: "unknown",
					MaxWidth:   1,
					Link:       "",
				},
			},
		},
	}
	org := "kubernetes"
	repo := "test-infra"
	for _, tc := range cases {
		updateCommitData(tc.before, org, repo, tc.hash, tc.buildTime, tc.width)
		for hash, expCommit := range tc.after {
			if commit, ok := tc.before[hash]; ok {
				if commit.HashPrefix != expCommit.HashPrefix {
					t.Errorf("%s: expected commit hash prefix to be %s, got %s", tc.name, expCommit.HashPrefix, commit.HashPrefix)
				}
				if commit.Link != expCommit.Link {
					t.Errorf("%s: expected commit link to be %s, got %s", tc.name, expCommit.Link, commit.Link)
				}
				if commit.MaxWidth != expCommit.MaxWidth {
					t.Errorf("%s: expected commit width to be %d, got %d", tc.name, expCommit.MaxWidth, commit.MaxWidth)
				}
				if commit.latest != expCommit.latest {
					t.Errorf("%s: expected commit time to be %v, got %v", tc.name, expCommit.latest, commit.latest)
				}
			} else {
				t.Errorf("%s: expected commit %s not found", tc.name, hash)
			}
		}
	}
}

func TestParsePullKey(t *testing.T) {
	cases := []struct {
		name   string
		key    string
		org    string
		repo   string
		pr     int
		expErr bool
	}{
		{
			name: "all good",
			key:  "kubernetes/test-infra/10169",
			org:  "kubernetes",
			repo: "test-infra",
			pr:   10169,
		},
		{
			name:   "3rd field needs to be PR number",
			key:    "kubernetes/test-infra/alpha",
			expErr: true,
		},
		{
			name:   "not enough parts",
			key:    "kubernetes/10169",
			expErr: true,
		},
	}
	for _, tc := range cases {
		org, repo, pr, err := parsePullKey(tc.key)
		if (err != nil) != tc.expErr {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}
		if org != tc.org || repo != tc.repo || pr != tc.pr {
			t.Errorf("%s: expected %s, %s, %d; got %s, %s, %d", tc.name, tc.org, tc.repo, tc.pr, org, repo, pr)
		}
	}
}

var testBucket = fakeBucket{
	name: "chum-bucket",
	objects: map[string]string{
		"pr-logs/pull/123/build-snowman/456/started.json": `{
			"timestamp": 55555,
			"pull": "master:d0c3cd182cffb3e722b14322fd1ca854a8bf62b0,69848:1244ee66517bbe603d899bbd24458ebc0e185fd9"
		}`,
		"pr-logs/pull/123/build-snowman/456/finished.json": `{
			"timestamp": 66666,
			"result":    "SUCCESS"
		}`,
		"pr-logs/pull/123/build-snowman/789/started.json": `{
			"timestamp": 98765,
			"revision": "bbdebedaf24c03f9e2eeb88e8ea4bb10c9e1fbfc"
		}`,
		"pr-logs/pull/765/eat-bread/999/started.json": `{
			"timestamp": 12345,
			"pull": "not-master:21ebe05079a1aeb5f6dae23a2d8c106b4af8c363,12345:52252bcc81712c96940fca1d3c913dd76af3d2a2"
		}`,
	},
}

func TestListJobBuilds(t *testing.T) {
	jobPrefixes := []string{"pr-logs/pull/123/build-snowman/", "pr-logs/pull/765/eat-bread/"}
	expected := map[string]sets.String{
		"build-snowman": {"456": {}, "789": {}},
		"eat-bread":     {"999": {}},
	}
	jobs := listJobBuilds(testBucket, jobPrefixes)
	if len(jobs) != len(expected) {
		t.Errorf("expected %d jobs, got %d", len(expected), len(jobs))
	}
	for _, job := range jobs {
		if expBuilds, ok := expected[job.name]; ok {
			if len(job.buildPrefixes) != len(expBuilds) {
				t.Errorf("expected %d builds for %q, found %d", len(expBuilds), job.name, len(job.buildPrefixes))
			}
			for _, build := range job.buildPrefixes {
				if !expBuilds.Has(build) {
					t.Errorf("found unexpected build for %q: %q", job.name, build)
				}
			}
		} else {
			t.Errorf("found unexpected job %q", job.name)
		}
	}
}

func TestGetPRBuildData(t *testing.T) {
	jobs := []jobBuilds{
		{
			name: "build-snowman",
			buildPrefixes: []string{
				"pr-logs/pull/123/build-snowman/456",
				"pr-logs/pull/123/build-snowman/789",
			},
		},
		{
			name: "eat-bread",
			buildPrefixes: []string{
				"pr-logs/pull/765/eat-bread/999",
			},
		},
	}
	expected := map[string]buildData{
		"pr-logs/pull/123/build-snowman/456": {
			prefix:       "pr-logs/pull/123/build-snowman/456",
			jobName:      "build-snowman",
			index:        0,
			ID:           "456",
			SpyglassLink: "/view/gcs/chum-bucket/pr-logs/pull/123/build-snowman/456",
			Started:      time.Unix(55555, 0),
			Duration:     time.Unix(66666, 0).Sub(time.Unix(55555, 0)),
			Result:       "SUCCESS",
			commitHash:   "1244ee66517bbe603d899bbd24458ebc0e185fd9",
		},
		"pr-logs/pull/123/build-snowman/789": {
			prefix:       "pr-logs/pull/123/build-snowman/789",
			jobName:      "build-snowman",
			index:        1,
			ID:           "789",
			SpyglassLink: "/view/gcs/chum-bucket/pr-logs/pull/123/build-snowman/789",
			Started:      time.Unix(98765, 0),
			Result:       "Unknown",
			commitHash:   "bbdebedaf24c03f9e2eeb88e8ea4bb10c9e1fbfc",
		},
		"pr-logs/pull/765/eat-bread/999": {
			prefix:       "pr-logs/pull/765/eat-bread/999",
			jobName:      "eat-bread",
			index:        0,
			ID:           "999",
			SpyglassLink: "/view/gcs/chum-bucket/pr-logs/pull/765/eat-bread/999",
			Started:      time.Unix(12345, 0),
			Result:       "Unknown",
			commitHash:   "52252bcc81712c96940fca1d3c913dd76af3d2a2",
		},
	}
	builds := getPRBuildData(testBucket, jobs)
	if len(builds) != len(expected) {
		t.Errorf("expected %d builds, found %d", len(expected), len(builds))
	}
	for _, build := range builds {
		if expBuild, ok := expected[build.prefix]; ok {
			if !reflect.DeepEqual(build, expBuild) {
				t.Errorf("build %s mismatch:\n%s", build.prefix, diff.ObjectReflectDiff(expBuild, build))
			}
		} else {
			t.Errorf("found unexpected build %s", build.prefix)
		}
	}
}

func TestGetGCSDirsForPR(t *testing.T) {
	cases := []struct {
		name     string
		expected map[string][]string
		config   *config.Config
		org      string
		repo     string
		pr       int
		expErr   bool
	}{
		{
			name:   "no presubmits",
			org:    "kubernetes",
			repo:   "fizzbuzz",
			pr:     123,
			config: &config.Config{},
			expErr: true,
		},
		{
			name: "multiple buckets",
			expected: map[string][]string{
				"chum-bucket": {
					"pr-logs/pull/prow/123/",
				},
				"krusty-krab": {
					"pr-logs/pull/prow/123/",
				},
			},
			org:  "kubernetes",
			repo: "prow", // someday
			pr:   123,
			config: &config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{
						DefaultDecorationConfig: &kube.DecorationConfig{
							GCSConfiguration: &kube.GCSConfiguration{
								Bucket:       "krusty-krab",
								PathStrategy: "legacy",
								DefaultOrg:   "kubernetes",
								DefaultRepo:  "kubernetes",
							},
						},
					},
				},
				JobConfig: config.JobConfig{
					Presubmits: map[string][]config.Presubmit{
						"kubernetes/prow": {
							{
								JobBase: config.JobBase{
									Name: "fum-is-chum",
									UtilityConfig: config.UtilityConfig{
										DecorationConfig: &kube.DecorationConfig{
											GCSConfiguration: &kube.GCSConfiguration{
												Bucket:       "chum-bucket",
												PathStrategy: "legacy",
												DefaultOrg:   "kubernetes",
												DefaultRepo:  "kubernetes",
											},
										},
									},
								},
							},
							{
								JobBase: config.JobBase{
									Name: "protect-formula",
									// undecorated
								},
							},
						},
					},
				},
			},
		},
	}
	for _, tc := range cases {
		toSearch, err := getGCSDirsForPR(tc.config, tc.org, tc.repo, tc.pr)
		if (err != nil) != tc.expErr {
			t.Errorf("%s: unexpected error %v", tc.name, err)
		}
		for bucket, expDirs := range tc.expected {
			if dirs, ok := toSearch[bucket]; ok {
				if len(dirs) != len(expDirs) {
					t.Errorf("expected to find %d dirs in bucket %s, found %d", len(expDirs), bucket, len(dirs))
				}
				for _, expDir := range tc.expected[bucket] {
					if !dirs.Has(expDir) {
						t.Errorf("couldn't find expected dir %s in bucket %s", expDir, bucket)
					}
				}
			} else {
				t.Errorf("expected to find %d dirs in bucket %s, found none", len(expDirs), bucket)
			}
		}
	}
}
