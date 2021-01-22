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
	"context"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fsouza/fake-gcs-server/fakestorage"

	"k8s.io/test-infra/prow/io"

	"github.com/google/go-cmp/cmp"
	"k8s.io/apimachinery/pkg/util/sets"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/github"
	"k8s.io/test-infra/prow/github/fakegithub"
	"k8s.io/test-infra/prow/io/providers"
)

type fakeBucket struct {
	name            string
	storageProvider string
	objects         map[string]string
}

func (bucket fakeBucket) getName() string {
	return bucket.name
}

func (bucket fakeBucket) getStorageProvider() string {
	return bucket.storageProvider
}

func (bucket fakeBucket) listSubDirs(_ context.Context, prefix string) ([]string, error) {
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

func (bucket fakeBucket) listAll(_ context.Context, prefix string) ([]string, error) {
	keys := []string{}
	for k := range bucket.objects {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		keys = append(keys, k)
	}
	return keys, nil
}

func (bucket fakeBucket) readObject(_ context.Context, key string) ([]byte, error) {
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
		updateCommitData(tc.before, "github.com", org, repo, tc.hash, tc.buildTime, tc.width)
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

func TestGetPullCommitHash(t *testing.T) {
	cases := []struct {
		pull       string
		commitHash string
		expErr     bool
	}{
		{
			pull:       "main:4fe6d226e0455ef3d16c1f639a4010d699d0d097,21354:6cf03d53a14f6287d2175b0e9f3fbb31d91981a7",
			commitHash: "6cf03d53a14f6287d2175b0e9f3fbb31d91981a7",
		},
		{
			pull:       "release45-v8.0:5b30685f6bbf7a0bfef3fa8f2ebe2626ec1df391,54884:d1e309d8d10388000a34b1f705fd78c648ea5faa",
			commitHash: "d1e309d8d10388000a34b1f705fd78c648ea5faa",
		},
		{
			pull:   "main:6c1db48d6911675873b25457dbe61adca0d428a0,pullre:4905771e4f06c00385d7b1ac3c6de76f173e0212",
			expErr: true,
		},
		{
			pull:   "23545",
			expErr: true,
		},
		{
			pull:   "main:6c1db48d6911675873b25457dbe61adca0d428a0,12354:548461",
			expErr: true,
		},
		{
			pull:   "main:6c1db48d6,12354:e3e9d3eaa3a43f0a4fac47eccd379f077bee6789",
			expErr: true,
		},
	}

	for _, tc := range cases {
		commitHash, err := getPullCommitHash(tc.pull)
		if (err != nil) != tc.expErr {
			t.Errorf("%q: unexpected error: %v", tc.pull, err)
			continue
		}
		if commitHash != tc.commitHash {
			t.Errorf("%s: expected commit hash to be '%s', got '%s'", tc.pull, tc.commitHash, commitHash)
		}
	}
}

func TestParsePullURL(t *testing.T) {
	cases := []struct {
		name   string
		addr   string
		org    string
		repo   string
		pr     int
		expErr bool
	}{
		{
			name: "simple org/repo",
			addr: "https://prow.k8s.io/pr-history?org=kubernetes&repo=test-infra&pr=10169",
			org:  "kubernetes",
			repo: "test-infra",
			pr:   10169,
		},
		{
			name: "Gerrit org/repo",
			addr: "https://prow.k8s.io/pr-history?org=http://theponyapi.com&repo=test/ponies&pr=12345",
			org:  "http://theponyapi.com",
			repo: "test/ponies",
			pr:   12345,
		},
		{
			name:   "PR needs to be an int",
			addr:   "https://prow.k8s.io/pr-history?org=kubernetes&repo=test-infra&pr=alpha",
			expErr: true,
		},
		{
			name:   "missing org",
			addr:   "https://prow.k8s.io/pr-history?repo=test-infra&pr=10169",
			expErr: true,
		},
		{
			name:   "missing repo",
			addr:   "https://prow.k8s.io/pr-history?org=kubernetes&pr=10169",
			expErr: true,
		},
		{
			name:   "missing pr",
			addr:   "https://prow.k8s.io/pr-history?org=kubernetes&repo=test-infra",
			expErr: true,
		},
	}
	for _, tc := range cases {
		u, err := url.Parse(tc.addr)
		if err != nil {
			t.Errorf("bad test URL %s: %v", tc.addr, err)
			continue
		}
		org, repo, pr, err := parsePullURL(u)
		if (err != nil) != tc.expErr {
			t.Errorf("%q: unexpected error: %v", tc.name, err)
		}
		if org != tc.org || repo != tc.repo || pr != tc.pr {
			t.Errorf("%q: expected %s, %s, %d; got %s, %s, %d", tc.name, tc.org, tc.repo, tc.pr, org, repo, pr)
		}
	}
}

var testBucket = fakeBucket{
	name:            "chum-bucket",
	storageProvider: providers.GS,
	objects: map[string]string{
		"pr-logs/pull/123/build-snowman/456/started.json": `{
			"timestamp": 55555
		}`,
		"pr-logs/pull/123/build-snowman/456/finished.json": `{
			"timestamp": 66666,
			"result":    "SUCCESS",
			"revision":  "1244ee66517bbe603d899bbd24458ebc0e185fd9"
		}`,
		"pr-logs/pull/123/build-snowman/789/started.json": `{
			"timestamp": 98765,
			"pull": "master:d0c3cd182cffb3e722b14322fd1ca854a8bf62b0,69848:bbdebedaf24c03f9e2eeb88e8ea4bb10c9e1fbfc"
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
	jobs := listJobBuilds(context.Background(), testBucket, jobPrefixes)
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
	expected := map[string]struct {
		fixedDuration bool
		buildData     buildData
	}{
		"pr-logs/pull/123/build-snowman/456": {
			fixedDuration: true,
			buildData: buildData{
				prefix:       "pr-logs/pull/123/build-snowman/456",
				jobName:      "build-snowman",
				index:        0,
				ID:           "456",
				SpyglassLink: "/view/gs/chum-bucket/pr-logs/pull/123/build-snowman/456",
				Started:      time.Unix(55555, 0),
				Duration:     time.Unix(66666, 0).Sub(time.Unix(55555, 0)),
				Result:       "SUCCESS",
				commitHash:   "1244ee66517bbe603d899bbd24458ebc0e185fd9",
			},
		},
		"pr-logs/pull/123/build-snowman/789": {
			buildData: buildData{
				prefix:       "pr-logs/pull/123/build-snowman/789",
				jobName:      "build-snowman",
				index:        1,
				ID:           "789",
				SpyglassLink: "/view/gs/chum-bucket/pr-logs/pull/123/build-snowman/789",
				Started:      time.Unix(98765, 0),
				Result:       "Pending",
				commitHash:   "bbdebedaf24c03f9e2eeb88e8ea4bb10c9e1fbfc",
			},
		},
		"pr-logs/pull/765/eat-bread/999": {
			buildData: buildData{
				prefix:       "pr-logs/pull/765/eat-bread/999",
				jobName:      "eat-bread",
				index:        0,
				ID:           "999",
				SpyglassLink: "/view/gs/chum-bucket/pr-logs/pull/765/eat-bread/999",
				Started:      time.Unix(12345, 0),
				Result:       "Pending",
				commitHash:   "52252bcc81712c96940fca1d3c913dd76af3d2a2",
			},
		},
	}
	builds := getPRBuildData(context.Background(), testBucket, jobs)
	if len(builds) != len(expected) {
		t.Errorf("expected %d builds, found %d", len(expected), len(builds))
	}
	cmpOption := cmp.AllowUnexported(buildData{})
	for _, build := range builds {
		if exp, ok := expected[build.prefix]; ok {
			if !exp.fixedDuration {
				build.Duration = 0
			}

			if diff := cmp.Diff(build, exp.buildData, cmpOption); diff != "" {
				t.Errorf("build %s mismatch (-got, +want):\n%s", build.prefix, diff)
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
				"gs://chum-bucket": {
					"pr-logs/pull/prow/123/",
				},
				"gs://krusty-krab": {
					"pr-logs/pull/prow/123/",
				},
			},
			org:  "kubernetes",
			repo: "prow", // someday
			pr:   123,
			config: &config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{
						DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
							"*": {
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "krusty-krab",
									PathStrategy: "legacy",
									DefaultOrg:   "kubernetes",
									DefaultRepo:  "kubernetes",
								},
							},
						},
					},
				},
				JobConfig: config.JobConfig{
					PresubmitsStatic: map[string][]config.Presubmit{
						"kubernetes/prow": {
							{
								JobBase: config.JobBase{
									Name: "fum-is-chum",
									UtilityConfig: config.UtilityConfig{
										DecorationConfig: &prowapi.DecorationConfig{
											GCSConfiguration: &prowapi.GCSConfiguration{
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
		gitHubClient := fakegithub.NewFakeClient()
		gitHubClient.PullRequests = map[int]*github.PullRequest{
			123: {Number: 123},
		}
		toSearch, err := getStorageDirsForPR(tc.config, gitHubClient, nil, tc.org, tc.repo, tc.pr)
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

func Test_getPRHistory(t *testing.T) {
	c := &config.Config{
		JobConfig: config.JobConfig{
			PresubmitsStatic: map[string][]config.Presubmit{
				"kubernetes/test-infra": {
					{
						JobBase: config.JobBase{
							Name: "pull-test-infra-bazel",
							UtilityConfig: config.UtilityConfig{
								DecorationConfig: &prowapi.DecorationConfig{
									GCSConfiguration: &prowapi.GCSConfiguration{
										Bucket:       "kubernetes-jenkins",
										PathStrategy: prowapi.PathStrategyLegacy,
										DefaultOrg:   "kubernetes",
									},
								},
							},
						},
					},
					{
						JobBase: config.JobBase{
							Name: "pull-test-infra-yamllint",
						},
					},
				},
			},
		},
		ProwConfig: config.ProwConfig{
			Plank: config.Plank{
				DefaultDecorationConfigs: map[string]*prowapi.DecorationConfig{
					"*": {
						GCSConfiguration: &prowapi.GCSConfiguration{
							Bucket:       "gs://kubernetes-jenkins",
							PathStrategy: prowapi.PathStrategyLegacy,
							DefaultOrg:   "kubernetes",
						},
					},
				},
			},
			Deck: config.Deck{
				AllKnownStorageBuckets: sets.NewString("kubernetes-jenkins"),
			},
		},
	}
	objects := []fakestorage.Object{
		{
			BucketName: "kubernetes-jenkins",
			Name:       "pr-logs/pull/test-infra/17183/pull-test-infra-bazel/1254406011708510210/started.json",
			Content:    []byte("{\"timestamp\": 1587908709,\"pull\": \"17183\",\"repos\": {\"kubernetes/test-infra\": \"master:48192e9a938ed25edb646de2ee9b4ec096c02732,17183:664ba002bc2155e7438b810a1bb7473c55dc1c6a\"},\"metadata\": {\"resultstore\": \"https://source.cloud.google.com/results/invocations/8edcebc7-11f3-4c4e-a7c3-cae6d26bd117/targets/test\"},\"repo-version\": \"a31d10b2924182638acad0f4b759f53e73b5f817\",\"Pending\": false}"),
		},
		{
			BucketName: "kubernetes-jenkins",
			Name:       "pr-logs/pull/test-infra/17183/pull-test-infra-bazel/1254406011708510210/finished.json",
			Content:    []byte("{\"timestamp\": 1587909145,\"passed\": true,\"result\": \"SUCCESS\",\"revision\": \"664ba002bc2155e7438b810a1bb7473c55dc1c6a\"}"),
		},
		{
			BucketName: "kubernetes-jenkins",
			Name:       "pr-logs/pull/test-infra/17183/pull-test-infra-yamllint/1254406011708510208/started.json",
			Content:    []byte("{\"timestamp\": 1587908749,\"pull\": \"17183\",\"repos\": {\"kubernetes/test-infra\": \"master:48192e9a938ed25edb646de2ee9b4ec096c02732,17183:664ba002bc2155e7438b810a1bb7473c55dc1c6a\"},\"metadata\": {\"resultstore\": \"https://source.cloud.google.com/results/invocations/af70141d-0990-4e63-9ebf-db874391865e/targets/test\"},\"repo-version\": \"a31d10b2924182638acad0f4b759f53e73b5f817\",\"Pending\": false}"),
		},
		{
			BucketName: "kubernetes-jenkins",
			Name:       "pr-logs/pull/test-infra/17183/pull-test-infra-yamllint/1254406011708510208/finished.json",
			Content:    []byte("{\"timestamp\": 1587908767,\"passed\": true,\"result\": \"SUCCESS\",\"revision\": \"664ba002bc2155e7438b810a1bb7473c55dc1c6a\"}"),
		},
	}
	gcsServer := fakestorage.NewServer(objects)
	defer gcsServer.Stop()

	fakeGCSClient := gcsServer.Client()

	wantedPRHistory := prHistoryTemplate{
		Link: "https://github.com/kubernetes/test-infra/pull/17183",
		Name: "kubernetes/test-infra #17183",
		Jobs: []prJobData{
			{
				Name: "pull-test-infra-bazel",
				Link: "/job-history/gs/kubernetes-jenkins/pr-logs/directory/pull-test-infra-bazel",
				Builds: []buildData{
					{
						index:        0,
						jobName:      "pull-test-infra-bazel",
						prefix:       "pr-logs/pull/test-infra/17183/pull-test-infra-bazel/1254406011708510210/",
						SpyglassLink: "/view/gs/kubernetes-jenkins/pr-logs/pull/test-infra/17183/pull-test-infra-bazel/1254406011708510210",
						ID:           "1254406011708510210",
						Started:      time.Unix(1587908709, 0),
						Duration:     436000000000,
						Result:       "SUCCESS",
						commitHash:   "664ba002bc2155e7438b810a1bb7473c55dc1c6a",
					},
				},
			},
			{
				Name: "pull-test-infra-yamllint",
				Link: "/job-history/gs/kubernetes-jenkins/pr-logs/directory/pull-test-infra-yamllint",
				Builds: []buildData{
					{
						index:        0,
						jobName:      "pull-test-infra-yamllint",
						prefix:       "pr-logs/pull/test-infra/17183/pull-test-infra-yamllint/1254406011708510208/",
						SpyglassLink: "/view/gs/kubernetes-jenkins/pr-logs/pull/test-infra/17183/pull-test-infra-yamllint/1254406011708510208",
						ID:           "1254406011708510208",
						Started:      time.Unix(1587908749, 0),
						Duration:     18000000000,
						Result:       "SUCCESS",
						commitHash:   "664ba002bc2155e7438b810a1bb7473c55dc1c6a",
					},
				},
			},
		},
		Commits: []commitData{
			{
				Hash:       "664ba002bc2155e7438b810a1bb7473c55dc1c6a",
				HashPrefix: "664ba00",
				Link:       "https://github.com/kubernetes/test-infra/commit/664ba002bc2155e7438b810a1bb7473c55dc1c6a",
				MaxWidth:   1,
				latest:     time.Unix(1587908749, 0),
			},
		},
	}

	type args struct {
		url string
	}
	tests := []struct {
		name    string
		args    args
		want    prHistoryTemplate
		wantErr bool
	}{
		{
			name: "get pr history",
			args: args{
				url: "https://prow.k8s.io/pr-history/?org=kubernetes&repo=test-infra&pr=17183",
			},
			want: wantedPRHistory,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prHistoryURL, _ := url.Parse(tt.args.url)
			got, err := getPRHistory(context.Background(), prHistoryURL, c, io.NewGCSOpener(fakeGCSClient), nil, nil, "github.com")
			if (err != nil) != tt.wantErr {
				t.Errorf("getPRHistory() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getPRHistory() got = %v, want %v", got, tt.want)
			}
		})
	}
}
