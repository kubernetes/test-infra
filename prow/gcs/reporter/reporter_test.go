/*
Copyright 2020 The Kubernetes Authors.

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

package reporter

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/testgrid/metadata"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	prowv1 "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
)

var (
	fakeGCSServer *fakestorage.Server
)

type fca struct {
	c config.Config
}

func (ca fca) Config() *config.Config {
	return &ca.c
}

func sampleProwjob() prowv1.ProwJob {
	return prowv1.ProwJob{
		Spec: prowv1.ProwJobSpec{
			Type: prowv1.PresubmitJob,
			Refs: &prowv1.Refs{
				Org:   "kubernetes",
				Repo:  "test-infra",
				Pulls: []prowv1.Pull{{Number: 12345}},
			},
			Agent: prowv1.KubernetesAgent,
			Job:   "my-little-job",
		},
		Status: prowv1.ProwJobStatus{
			State:     prowv1.TriggeredState,
			StartTime: metav1.Time{Time: time.Date(2010, 10, 10, 18, 30, 0, 0, time.UTC)},
			PodName:   "some-pod",
			BuildID:   "123",
		},
	}
}

func sampleConfig() config.Getter {
	return fca{c: config.Config{
		ProwConfig: config.ProwConfig{
			Plank: config.Plank{
				DefaultDecorationConfigs: map[string]*prowv1.DecorationConfig{"*": {
					GCSConfiguration: &prowv1.GCSConfiguration{
						Bucket:       "kubernetes-jenkins",
						PathPrefix:   "some-prefix",
						PathStrategy: prowv1.PathStrategyLegacy,
						DefaultOrg:   "kubernetes",
						DefaultRepo:  "kubernetes",
					},
				}},
			},
		},
	}}.Config
}

func TestMain(m *testing.M) {
	var longLog string
	for i := 0; i < 300; i++ {
		longLog += "here a log\nthere a log\neverywhere a log log\n"
	}
	fakeGCSServer = fakestorage.NewServer([]fakestorage.Object{})
	fakeGCSServer.CreateBucket("kubernetes-jenkins")
	os.Exit(m.Run())
}

func TestUploadProwjob(t *testing.T) {
	cfg := sampleConfig()
	pj := sampleProwjob()

	s := fakeGCSServer.Client()
	gr := New(cfg, s, false)
	ctx := context.Background()

	err := gr.reportProwjob(ctx, &pj)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	serialised, err := json.Marshal(pj)
	if err != nil {
		t.Fatalf("Unexpected error serialising prowjob: %v", err)
	}

	objPath := "some-prefix/pr-logs/pull/test-infra/12345/my-little-job/123/prowjob.json"
	obj, err := fakeGCSServer.GetObject("kubernetes-jenkins", objPath)
	if err != nil {
		t.Fatalf("Couldn't fetch expected file from %s: %v", objPath, err)
	}

	if !bytes.Equal(obj.Content, serialised) {
		t.Fatalf("Serialised content does not match:\n\nExpected:\n%q\n\nActual:\n%q", string(serialised), string(obj.Content))
	}
}

func TestUploadStartedJson(t *testing.T) {
	s := fakeGCSServer.Client()
	cfg := sampleConfig()
	pj := sampleProwjob()

	gr := New(cfg, s, false)

	ctx := context.Background()
	err := gr.reportStartedJob(ctx, &pj)
	if err != nil {
		t.Fatalf("Unexpected error reporting job: %v", err)
	}

	objPath := "some-prefix/pr-logs/pull/test-infra/12345/my-little-job/123/started.json"
	obj, err := fakeGCSServer.GetObject("kubernetes-jenkins", objPath)
	if err != nil {
		t.Fatalf("Couldn't fetch expected file from %v: %v", objPath, err)
	}
	var result metadata.Started
	err = json.Unmarshal(obj.Content, &result)
	if err != nil {
		t.Fatalf("Couldn't unmarshal started.json: %v. Content: %q", err, string(obj.Content))
	}
	if result.Timestamp != pj.Status.StartTime.Unix() {
		t.Fatalf("started.json does not have the expected timestamp (got %d, expected %d)", result.Timestamp, pj.Status.StartTime.Unix())
	}
}

func TestUploadFinishedJson(t *testing.T) {
	s := fakeGCSServer.Client()
	cfg := sampleConfig()
	pj := sampleProwjob()
	pj.Status.State = prowv1.SuccessState
	pj.Status.CompletionTime = &metav1.Time{Time: time.Date(2019, 10, 13, 0, 0, 0, 0, time.UTC)}

	gr := New(cfg, s, false)

	ctx := context.Background()
	err := gr.reportFinishedJob(ctx, &pj)
	if err != nil {
		t.Fatalf("Unexpected error reporting job: %v", err)
	}

	objPath := "some-prefix/pr-logs/pull/test-infra/12345/my-little-job/123/finished.json"
	obj, err := fakeGCSServer.GetObject("kubernetes-jenkins", objPath)
	if err != nil {
		t.Fatalf("Couldn't fetch expected file from %v: %v", objPath, err)
	}
	var result metadata.Finished
	err = json.Unmarshal(obj.Content, &result)
	if err != nil {
		t.Fatalf("Couldn't unmarshal finished.json: %v. Content: %q", err, string(obj.Content))
	}
	if result.Timestamp == nil {
		t.Fatalf("finished.json does not have a finish timestamp.")
	}
	if *result.Timestamp != pj.Status.CompletionTime.Unix() {
		t.Fatalf("finished.json does not have the expected timestamp (got %d, expected %d)", result.Timestamp, pj.Status.CompletionTime.Unix())
	}
}
