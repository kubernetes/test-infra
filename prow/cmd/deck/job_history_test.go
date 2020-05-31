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
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/fsouza/fake-gcs-server/fakestorage"

	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/io/providers"
)

func TestJobHistURL(t *testing.T) {
	cases := []struct {
		name            string
		address         string
		storageProvider string
		bktName         string
		root            string
		id              int64
		expErr          bool
	}{
		{
			address:         "http://www.example.com/job-history/foo-bucket/logs/bar-e2e",
			bktName:         "foo-bucket",
			storageProvider: providers.GS,
			root:            "logs/bar-e2e",
			id:              emptyID,
		},
		{
			address:         "http://www.example.com/job-history/foo-bucket/logs/bar-e2e?buildId=",
			bktName:         "foo-bucket",
			storageProvider: providers.GS,
			root:            "logs/bar-e2e",
			id:              emptyID,
		},
		{
			address:         "http://www.example.com/job-history/foo-bucket/logs/bar-e2e?buildId=123456789123456789",
			bktName:         "foo-bucket",
			storageProvider: providers.GS,
			root:            "logs/bar-e2e",
			id:              123456789123456789,
		},
		{
			address:         "http://www.example.com/job-history/gs/foo-bucket/logs/bar-e2e",
			bktName:         "foo-bucket",
			storageProvider: providers.GS,
			root:            "logs/bar-e2e",
			id:              emptyID,
		},
		{
			address:         "http://www.example.com/job-history/gs/foo-bucket/logs/bar-e2e?buildId=",
			bktName:         "foo-bucket",
			storageProvider: providers.GS,
			root:            "logs/bar-e2e",
			id:              emptyID,
		},
		{
			address:         "http://www.example.com/job-history/gs/foo-bucket/logs/bar-e2e?buildId=123456789123456789",
			bktName:         "foo-bucket",
			storageProvider: providers.GS,
			root:            "logs/bar-e2e",
			id:              123456789123456789,
		},
		{
			address:         "http://www.example.com/job-history/s3/foo-bucket/logs/bar-e2e",
			bktName:         "foo-bucket",
			storageProvider: providers.S3,
			root:            "logs/bar-e2e",
			id:              emptyID,
		},
		{
			address:         "http://www.example.com/job-history/s3/foo-bucket/logs/bar-e2e?buildId=",
			bktName:         "foo-bucket",
			storageProvider: providers.S3,
			root:            "logs/bar-e2e",
			id:              emptyID,
		},
		{
			address:         "http://www.example.com/job-history/s3/foo-bucket/logs/bar-e2e?buildId=123456789123456789",
			bktName:         "foo-bucket",
			storageProvider: providers.S3,
			root:            "logs/bar-e2e",
			id:              123456789123456789,
		},
		{
			address: "http://www.example.com/job-history",
			expErr:  true,
		},
		{
			address: "http://www.example.com/job-history/",
			expErr:  true,
		},
		{
			address: "http://www.example.com/job-history/foo-bucket",
			expErr:  true,
		},
		{
			address: "http://www.example.com/job-history/foo-bucket/",
			expErr:  true,
		},
		{
			address: "http://www.example.com/job-history/foo-bucket/logs/bar-e2e?buildId=-738",
			expErr:  true,
		},
		{
			address: "http://www.example.com/job-history/foo-bucket/logs/bar-e2e?buildId=nope",
			expErr:  true,
		},
	}
	for _, tc := range cases {
		u, _ := url.Parse(tc.address)
		storageProvider, bktName, root, id, err := parseJobHistURL(u)
		if tc.expErr {
			if err == nil && tc.expErr {
				t.Errorf("parsing %q: expected error", tc.address)
			}
			continue
		}
		if err != nil {
			t.Errorf("parsing %q: unexpected error: %v", tc.address, err)
		}
		if storageProvider != tc.storageProvider {
			t.Errorf("parsing %q: expected storageProvider %s, got %s", tc.address, tc.storageProvider, storageProvider)
		}
		if bktName != tc.bktName {
			t.Errorf("parsing %q: expected bucket %s, got %s", tc.address, tc.bktName, bktName)
		}
		if root != tc.root {
			t.Errorf("parsing %q: expected root %s, got %s", tc.address, tc.root, root)
		}
		if id != tc.id {
			t.Errorf("parsing %q: expected id %d, got %d", tc.address, tc.id, id)
		}
	}
}

func eq(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCropResults(t *testing.T) {
	cases := []struct {
		a   []int64
		max int64
		exp []int64
		p   int
		q   int
	}{
		{
			a:   []int64{},
			max: 42,
			exp: []int64{},
			p:   -1,
			q:   0,
		},
		{
			a:   []int64{81, 27, 9, 3, 1},
			max: 100,
			exp: []int64{81, 27, 9, 3, 1},
			p:   0,
			q:   4,
		},
		{
			a:   []int64{81, 27, 9, 3, 1},
			max: 50,
			exp: []int64{27, 9, 3, 1},
			p:   1,
			q:   4,
		},
		{
			a:   []int64{25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1},
			max: 23,
			exp: []int64{23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4},
			p:   2,
			q:   21,
		},
	}
	for _, tc := range cases {
		actual, firstIndex, lastIndex := cropResults(tc.a, tc.max)
		if !eq(actual, tc.exp) || firstIndex != tc.p || lastIndex != tc.q {
			t.Errorf("cropResults(%v, %d) expected (%v, %d, %d), got (%v, %d, %d)",
				tc.a, tc.max, tc.exp, tc.p, tc.q, actual, firstIndex, lastIndex)
		}
	}
}

func TestLinkID(t *testing.T) {
	cases := []struct {
		startAddr string
		id        int64
		expAddr   string
	}{
		{
			startAddr: "http://www.example.com/job-history/foo-bucket/logs/bar-e2e",
			id:        -1,
			expAddr:   "http://www.example.com/job-history/foo-bucket/logs/bar-e2e?buildId=",
		},
		{
			startAddr: "http://www.example.com/job-history/foo-bucket/logs/bar-e2e",
			id:        23,
			expAddr:   "http://www.example.com/job-history/foo-bucket/logs/bar-e2e?buildId=23",
		},
	}
	for _, tc := range cases {
		u, _ := url.Parse(tc.startAddr)
		actual := linkID(u, tc.id)
		if actual != tc.expAddr {
			t.Errorf("adding id param %d expected %s, got %s", tc.id, tc.expAddr, actual)
		}
		again, _ := url.Parse(tc.startAddr)
		if again.String() != u.String() {
			t.Errorf("linkID incorrectly mutated URL (expected %s, got %s)", u.String(), again.String())
		}
	}
}

func Test_getJobHistory(t *testing.T) {
	objects := []fakestorage.Object{
		// pr-logs
		{
			BucketName: "kubernetes-jenkins",
			Name:       "pr-logs/directory/pull-test-infra-bazel/latest-build.txt",
			Content:    []byte("1254406011708510210"),
		},
		{
			BucketName: "kubernetes-jenkins",
			Name:       "pr-logs/directory/pull-test-infra-bazel/1221704015146913792.txt",
			Content:    []byte("gs://kubernetes-jenkins/pr-logs/pull/test-infra/16031/pull-test-infra-bazel/1221704015146913792"),
		},
		{
			BucketName: "kubernetes-jenkins",
			Name:       "pr-logs/pull/test-infra/16031/pull-test-infra-bazel/1221704015146913792/started.json",
			Content:    []byte("{\"timestamp\": 1580111939,\"pull\": \"16031\",\"repo-version\": \"19d9f301988f45d41addec0e307587addedbafdd\",\"repos\": {\"kubernetes/test-infra\": \"master:589aceb353f25b6af6f576f58ba16c71ef8870f3,16031:ec9156a00793375b5ca885b9b1f26be789315c50\"}}"),
		},
		{
			BucketName: "kubernetes-jenkins",
			Name:       "pr-logs/pull/test-infra/16031/pull-test-infra-bazel/1221704015146913792/finished.json",
			Content:    []byte("{\"timestamp\": 1580112259,\"passed\": true,\"result\": \"SUCCESS\",\"revision\": \"ec9156a00793375b5ca885b9b1f26be789315c50\"}"),
		},
		{
			BucketName: "kubernetes-jenkins",
			Name:       "pr-logs/directory/pull-test-infra-bazel/1254406011708510210.txt",
			Content:    []byte("gs://kubernetes-jenkins/pr-logs/pull/test-infra/17183/pull-test-infra-bazel/1254406011708510210"),
		},
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
		// logs
		{
			BucketName: "kubernetes-jenkins",
			Name:       "logs/post-cluster-api-provider-openstack-push-images/latest-build.txt",
			Content:    []byte("1253687771944456193"),
		},
		{
			BucketName: "kubernetes-jenkins",
			Name:       "logs/post-cluster-api-provider-openstack-push-images/1253687771944456193/started.json",
			Content:    []byte("{\"timestamp\": 1587737470,\"repos\": {\"kubernetes-sigs/cluster-api-provider-openstack\": \"master:b62656cde943aef3bcd1a18064aecff8b0f30a0c\"},\"metadata\": {\"resultstore\": \"https://source.cloud.google.com/results/invocations/9dce789e-c400-4204-a46c-86a3a5fde6c3/targets/test\"},\"repo-version\": \"b62656cde943aef3bcd1a18064aecff8b0f30a0c\",\"Pending\": false}"),
		},
		{
			BucketName: "kubernetes-jenkins",
			Name:       "logs/post-cluster-api-provider-openstack-push-images/1253687771944456193/finished.json",
			Content:    []byte("{\"timestamp\": 1587738205,\"passed\": true,\"result\": \"SUCCESS\",\"revision\": \"b62656cde943aef3bcd1a18064aecff8b0f30a0c\"}"),
		},
	}
	wantedPRLogsJobHistoryTemplate := jobHistoryTemplate{
		Name:         "pr-logs/directory/pull-test-infra-bazel",
		ResultsShown: 2,
		ResultsTotal: 2,
		Builds: []buildData{
			{
				index:        0,
				SpyglassLink: "/view/gs/kubernetes-jenkins/pr-logs/pull/test-infra/17183/pull-test-infra-bazel/1254406011708510210",
				ID:           "1254406011708510210",
				Started:      time.Unix(1587908709, 0),
				Duration:     436000000000,
				Result:       "SUCCESS",
				commitHash:   "664ba002bc2155e7438b810a1bb7473c55dc1c6a",
			},
			{
				index:        1,
				SpyglassLink: "/view/gs/kubernetes-jenkins/pr-logs/pull/test-infra/16031/pull-test-infra-bazel/1221704015146913792",
				ID:           "1221704015146913792",
				Started:      time.Unix(1580111939, 0),
				Duration:     320000000000,
				Result:       "SUCCESS",
				commitHash:   "ec9156a00793375b5ca885b9b1f26be789315c50",
			},
		},
	}
	wantedLogsJobHistoryTemplate := jobHistoryTemplate{
		Name:         "logs/post-cluster-api-provider-openstack-push-images",
		ResultsShown: 1,
		ResultsTotal: 1,
		Builds: []buildData{
			{
				index:        0,
				SpyglassLink: "/view/gs/kubernetes-jenkins/logs/post-cluster-api-provider-openstack-push-images/1253687771944456193",
				ID:           "1253687771944456193",
				Started:      time.Unix(1587737470, 0),
				Duration:     735000000000,
				Result:       "SUCCESS",
				commitHash:   "b62656cde943aef3bcd1a18064aecff8b0f30a0c",
			},
		},
	}
	gcsServer := fakestorage.NewServer(objects)
	defer gcsServer.Stop()

	fakeGCSClient := gcsServer.Client()

	tests := []struct {
		name    string
		url     string
		want    jobHistoryTemplate
		wantErr string
	}{
		{
			name: "get job history pr-logs (old format)",
			url:  "https://prow.k8s.io/job-history/kubernetes-jenkins/pr-logs/directory/pull-test-infra-bazel",
			want: wantedPRLogsJobHistoryTemplate,
		},
		{
			name: "get job history pr-logs (new format)",
			url:  "https://prow.k8s.io/job-history/gs/kubernetes-jenkins/pr-logs/directory/pull-test-infra-bazel",
			want: wantedPRLogsJobHistoryTemplate,
		},
		{
			name: "get job history logs (old format)",
			url:  "https://prow.k8s.io/job-history/kubernetes-jenkins/logs/post-cluster-api-provider-openstack-push-images",
			want: wantedLogsJobHistoryTemplate,
		},
		{
			name: "get job history logs (new format)",
			url:  "https://prow.k8s.io/job-history/gs/kubernetes-jenkins/logs/post-cluster-api-provider-openstack-push-images",
			want: wantedLogsJobHistoryTemplate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jobURL, _ := url.Parse(tt.url)
			got, err := getJobHistory(context.Background(), jobURL, io.NewGCSOpener(fakeGCSClient))
			var actualErr string
			if err != nil {
				actualErr = err.Error()
			}
			if actualErr != tt.wantErr {
				t.Errorf("getJobHistory() error = %v, wantErr %v", actualErr, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getJobHistory() got = %v, want %v", got, tt.want)
			}
		})
	}
}
