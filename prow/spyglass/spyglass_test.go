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

package spyglass

import (
	"fmt"
	"os"
	"path"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/kube"
)

var (
	fakeGCSBucket    *storage.BucketHandle
	testAf           *GCSArtifactFetcher
	fakeJa           *jobs.JobAgent
	fakeGCSJobSource *GCSJobSource
	buildLogName     string
	startedName      string
	finishedName     string
)

const (
	testSrc = "gs://test-bucket/logs/example-ci-run/403"
)

type fkc []kube.ProwJob

func (f fkc) GetLog(pod string) ([]byte, error) {
	return nil, nil
}

func (f fkc) ListPods(selector string) ([]kube.Pod, error) {
	return nil, nil
}

func (f fkc) ListProwJobs(s string) ([]kube.ProwJob, error) {
	return f, nil
}

type fpkc string

func (f fpkc) GetLog(pod string) ([]byte, error) {
	if pod == "wowowow" || pod == "powowow" {
		return []byte(f), nil
	}
	return nil, fmt.Errorf("pod not found: %s", pod)
}

func TestMain(m *testing.M) {
	fakeGCSJobSource = NewGCSJobSource(testSrc)
	testBucketName := fakeGCSJobSource.BucketName()
	buildLogName = path.Join(fakeGCSJobSource.JobPath(), "build-log.txt")
	startedName = path.Join(fakeGCSJobSource.JobPath(), "started.json")
	finishedName = path.Join(fakeGCSJobSource.JobPath(), "finished.json")
	fakeGCSServer := fakestorage.NewServer([]fakestorage.Object{
		{
			BucketName: testBucketName,
			Name:       buildLogName,
			Content:    []byte("Oh wow\nlogs\nthis is\ncrazy"),
		},
		{
			BucketName: testBucketName,
			Name:       startedName,
			Content: []byte(`{
						  "node": "gke-prow-default-pool-3c8994a8-qfhg", 
						  "repo-version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "timestamp": 1528742858, 
						  "repos": {
						    "k8s.io/kubernetes": "master", 
						    "k8s.io/release": "master"
						  }, 
						  "version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "metadata": {
						    "pod": "cbc53d8e-6da7-11e8-a4ff-0a580a6c0269"
						  }
						}`),
		},
		{
			BucketName: testBucketName,
			Name:       finishedName,
			Content: []byte(`{
						  "timestamp": 1528742943, 
						  "version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "result": "SUCCESS", 
						  "passed": true, 
						  "job-version": "v1.12.0-alpha.0.985+e6f64d0a79243c", 
						  "metadata": {
						    "repo": "k8s.io/kubernetes", 
						    "repos": {
						      "k8s.io/kubernetes": "master", 
						      "k8s.io/release": "master"
						    }, 
						    "infra-commit": "260081852", 
						    "pod": "cbc53d8e-6da7-11e8-a4ff-0a580a6c0269", 
						    "repo-commit": "e6f64d0a79243c834babda494151fc5d66582240"
						  },
						},`),
		},
	})
	defer fakeGCSServer.Stop()
	fakeGCSClient := fakeGCSServer.Client()
	fakeGCSBucket = fakeGCSClient.Bucket(testBucketName)
	testAf = &GCSArtifactFetcher{
		client: fakeGCSClient,
	}
	kc := fkc{
		kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Agent: kube.KubernetesAgent,
				Job:   "job",
			},
			Status: kube.ProwJobStatus{
				PodName: "wowowow",
				BuildID: "123",
			},
		},
		kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Agent:   kube.KubernetesAgent,
				Job:     "jib",
				Cluster: "trusted",
			},
			Status: kube.ProwJobStatus{
				PodName: "powowow",
				BuildID: "123",
			},
		},
	}
	fakeJa = jobs.NewJobAgent(kc, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, &config.Agent{})
	fakeJa.Start()
	os.Exit(m.Run())
}
