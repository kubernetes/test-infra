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
	"strings"
	"testing"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/spyglass/lenses"
)

var (
	fakeJa        *jobs.JobAgent
	fakeGCSServer *fakestorage.Server
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

func (f fpkc) GetContainerLog(pod, container string) ([]byte, error) {
	if pod == "wowowow" || pod == "powowow" {
		return []byte(f), nil
	}
	return nil, fmt.Errorf("pod not found: %s", pod)
}

func (f fpkc) GetLogTail(pod, container string, n int64) ([]byte, error) {
	if pod == "wowowow" || pod == "powowow" {
		tailBytes := []byte(f)
		lenTailBytes := int64(len(tailBytes))
		if lenTailBytes < n {
			return tailBytes, nil
		}
		return tailBytes[lenTailBytes-n-1:], nil
	}
	return nil, fmt.Errorf("pod not found: %s", pod)
}

type fca struct {
	c config.Config
}

func (ca fca) Config() *config.Config {
	return &ca.c
}

func TestMain(m *testing.M) {
	var longLog string
	for i := 0; i < 300; i++ {
		longLog += "here a log\nthere a log\neverywhere a log log\n"
	}
	fakeGCSServer = fakestorage.NewServer([]fakestorage.Object{
		{
			BucketName: "test-bucket",
			Name:       "logs/example-ci-run/403/build-log.txt",
			Content:    []byte("Oh wow\nlogs\nthis is\ncrazy"),
		},
		{
			BucketName: "test-bucket",
			Name:       "logs/example-ci-run/403/long-log.txt",
			Content:    []byte(longLog),
		},
		{
			BucketName: "test-bucket",
			Name:       "logs/example-ci-run/403/junit_01.xml",
			Content: []byte(`<testsuite tests="1017" failures="1017" time="0.016981535">
<testcase name="BeforeSuite" classname="Kubernetes e2e suite" time="0.006343795">
<failure type="Failure">
test/e2e/e2e.go:137 BeforeSuite on Node 1 failed test/e2e/e2e.go:137
</failure>
</testcase>
</testsuite>`),
		},
		{
			BucketName: "test-bucket",
			Name:       "logs/example-ci-run/403/started.json",
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
			BucketName: "test-bucket",
			Name:       "logs/example-ci-run/403/finished.json",
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

type dumpLens struct{}

func (dumpLens) Name() string {
	return "dump"
}

func (dumpLens) Title() string {
	return "Dump View"
}

func (dumpLens) Priority() int {
	return 1
}

func (dumpLens) Header(artifacts []lenses.Artifact, resourceDir string) string {
	return ""
}

func (dumpLens) Body(artifacts []lenses.Artifact, resourceDir, data string) string {
	var view []byte
	for _, a := range artifacts {
		data, err := a.ReadAll()
		if err != nil {
			logrus.WithError(err).Error("Error reading artifact")
			continue
		}
		view = append(view, data...)
	}
	return string(view)
}

func (dumpLens) Callback(artifacts []lenses.Artifact, resourceDir, data string) string {
	return ""
}

func TestViews(t *testing.T) {
	fakeGCSClient := fakeGCSServer.Client()
	testCases := []struct {
		name               string
		registeredViewers  []lenses.Lens
		matchCache         map[string][]string
		expectedLensTitles []string
	}{
		{
			name:              "Spyglass basic test",
			registeredViewers: []lenses.Lens{dumpLens{}},
			matchCache: map[string][]string{
				"dump": {"started.json"},
			},
			expectedLensTitles: []string{"Dump View"},
		},
		{
			name:              "Spyglass no matches",
			registeredViewers: []lenses.Lens{dumpLens{}},
			matchCache: map[string][]string{
				"dump": {},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, l := range tc.registeredViewers {
				lenses.RegisterLens(l)
			}
			sg := New(fakeJa, &config.Agent{}, fakeGCSClient)
			lenses := sg.Lenses(tc.matchCache)
			for _, l := range lenses {
				var found bool
				for _, title := range tc.expectedLensTitles {
					if title == l.Title() {
						found = true
					}
				}
				if !found {
					t.Errorf("lens title %s not found in expected titles.", l.Title())
				}
			}
			for _, title := range tc.expectedLensTitles {
				var found bool
				for _, l := range lenses {
					if title == l.Title() {
						found = true
					}
				}
				if !found {
					t.Errorf("expected title %s not found in produced lenses.", title)
				}
			}
		})
	}
}

func TestSplitSrc(t *testing.T) {
	testCases := []struct {
		name       string
		src        string
		expKeyType string
		expKey     string
		expError   bool
	}{
		{
			name:     "empty string",
			src:      "",
			expError: true,
		},
		{
			name:     "missing key",
			src:      "gcs",
			expError: true,
		},
		{
			name:       "prow key",
			src:        "prowjob/example-job-name/123456",
			expKeyType: "prowjob",
			expKey:     "example-job-name/123456",
		},
		{
			name:       "gcs key",
			src:        "gcs/kubernetes-jenkins/pr-logs/pull/test-infra/0000/example-job-name/314159/",
			expKeyType: "gcs",
			expKey:     "kubernetes-jenkins/pr-logs/pull/test-infra/0000/example-job-name/314159/",
		},
	}
	for _, tc := range testCases {
		keyType, key, err := splitSrc(tc.src)
		if tc.expError && err == nil {
			t.Errorf("test %q expected error", tc.name)
		}
		if !tc.expError && err != nil {
			t.Errorf("test %q encountered unexpected error: %v", tc.name, err)
		}
		if keyType != tc.expKeyType || key != tc.expKey {
			t.Errorf("test %q: splitting src %q: Expected <%q, %q>, got <%q, %q>",
				tc.name, tc.src, tc.expKeyType, tc.expKey, keyType, key)
		}
	}
}

func TestJobPath(t *testing.T) {
	kc := fkc{
		kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Type: kube.PeriodicJob,
				Job:  "example-periodic-job",
				DecorationConfig: &kube.DecorationConfig{
					GCSConfiguration: &kube.GCSConfiguration{
						Bucket: "chum-bucket",
					},
				},
			},
			Status: kube.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "1111",
			},
		},
		kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Type: kube.PresubmitJob,
				Job:  "example-presubmit-job",
				DecorationConfig: &kube.DecorationConfig{
					GCSConfiguration: &kube.GCSConfiguration{
						Bucket: "chum-bucket",
					},
				},
			},
			Status: kube.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "2222",
			},
		},
		kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Type: kube.PresubmitJob,
				Job:  "undecorated-job",
			},
			Status: kube.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "1",
			},
		},
		kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Type:             kube.PresubmitJob,
				Job:              "missing-gcs-job",
				DecorationConfig: &kube.DecorationConfig{},
			},
			Status: kube.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "1",
			},
		},
	}
	fakeJa = jobs.NewJobAgent(kc, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, &config.Agent{})
	fakeJa.Start()
	testCases := []struct {
		name       string
		src        string
		expJobPath string
		expError   bool
	}{
		{
			name:       "non-presubmit job in GCS with trailing /",
			src:        "gcs/kubernetes-jenkins/logs/example-job-name/123/",
			expJobPath: "kubernetes-jenkins/logs/example-job-name",
		},
		{
			name:       "non-presubmit job in GCS without trailing /",
			src:        "gcs/kubernetes-jenkins/logs/example-job-name/123",
			expJobPath: "kubernetes-jenkins/logs/example-job-name",
		},
		{
			name:       "presubmit job in GCS with trailing /",
			src:        "gcs/kubernetes-jenkins/pr-logs/pull/test-infra/0000/example-job-name/314159/",
			expJobPath: "kubernetes-jenkins/pr-logs/directory/example-job-name",
		},
		{
			name:       "presubmit job in GCS without trailing /",
			src:        "gcs/kubernetes-jenkins/pr-logs/pull/test-infra/0000/example-job-name/314159",
			expJobPath: "kubernetes-jenkins/pr-logs/directory/example-job-name",
		},
		{
			name:       "non-presubmit Prow job",
			src:        "prowjob/example-periodic-job/1111",
			expJobPath: "chum-bucket/logs/example-periodic-job",
		},
		{
			name:       "Prow presubmit job",
			src:        "prowjob/example-presubmit-job/2222",
			expJobPath: "chum-bucket/pr-logs/directory/example-presubmit-job",
		},
		{
			name:     "nonexistent job",
			src:      "prowjob/example-periodic-job/0000",
			expError: true,
		},
		{
			name:     "invalid key type",
			src:      "oh/my/glob/drama/bomb",
			expError: true,
		},
		{
			name:     "invalid GCS path",
			src:      "gcs/kubernetes-jenkins/bad-path",
			expError: true,
		},
		{
			name:     "job missing decoration",
			src:      "prowjob/undecorated-job/1",
			expError: true,
		},
		{
			name:     "job missing GCS config",
			src:      "prowjob/missing-gcs-job/1",
			expError: true,
		},
	}
	for _, tc := range testCases {
		fakeGCSClient := fakeGCSServer.Client()
		sg := New(fakeJa, &config.Agent{}, fakeGCSClient)
		jobPath, err := sg.JobPath(tc.src)
		if tc.expError && err == nil {
			t.Errorf("test %q: JobPath(%q) expected error", tc.name, tc.src)
			continue
		}
		if !tc.expError && err != nil {
			t.Errorf("test %q: JobPath(%q) returned unexpected error %v", tc.name, tc.src, err)
			continue
		}
		if jobPath != tc.expJobPath {
			t.Errorf("test %q: JobPath(%q) expected %q, got %q", tc.name, tc.src, tc.expJobPath, jobPath)
		}
	}
}

func TestProwToGCS(t *testing.T) {
	kc := fkc{
		kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Job: "gubernator-job",
			},
			Status: kube.ProwJobStatus{
				URL:     "https://gubernator.example.com/build/some-bucket/gubernator-job/1111/",
				BuildID: "1111",
			},
		},
		kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Job: "spyglass-job",
			},
			Status: kube.ProwJobStatus{
				URL:     "https://prow.example.com/view/gcs/some-bucket/spyglass-job/2222/",
				BuildID: "2222",
			},
		},
	}
	fakeJa = jobs.NewJobAgent(kc, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, &config.Agent{})
	fakeJa.Start()

	testCases := []struct {
		name         string
		key          string
		configPrefix string
		expectedPath string
		expectError  bool
	}{
		{
			name:         "extraction from gubernator-like URL",
			key:          "gubernator-job/1111",
			configPrefix: "https://gubernator.example.com/build/",
			expectedPath: "some-bucket/gubernator-job/1111/",
			expectError:  false,
		},
		{
			name:         "extraction from spyglass-like URL",
			key:          "spyglass-job/2222",
			configPrefix: "https://prow.example.com/view/gcs/",
			expectedPath: "some-bucket/spyglass-job/2222/",
			expectError:  false,
		},
		{
			name:         "failed extraction from wrong URL",
			key:          "spyglass-job/1111",
			configPrefix: "https://gubernator.example.com/build/",
			expectedPath: "",
			expectError:  true,
		},
		{
			name:         "prefix longer than URL",
			key:          "spyglass-job/2222",
			configPrefix: strings.Repeat("!", 100),
			expectError:  true,
		},
	}

	for _, tc := range testCases {
		fakeGCSClient := fakeGCSServer.Client()
		fakeConfigAgent := fca{
			c: config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{
						JobURLPrefix: tc.configPrefix,
					},
				},
			},
		}
		sg := New(fakeJa, fakeConfigAgent, fakeGCSClient)

		p, err := sg.prowToGCS(tc.key)
		if err != nil && !tc.expectError {
			t.Errorf("test %q: unexpected error: %v", tc.key, err)
			continue
		}
		if err == nil && tc.expectError {
			t.Errorf("test %q: expected an error but instead got success and path '%s'", tc.key, p)
			continue
		}
		if p != tc.expectedPath {
			t.Errorf("test %q: expected '%s' but got '%s'", tc.key, tc.expectedPath, p)
		}
	}
}

func TestFetchArtifactsPodLog(t *testing.T) {
	kc := fkc{
		kube.ProwJob{
			Spec: kube.ProwJobSpec{
				Agent: kube.KubernetesAgent,
				Job:   "job",
			},
			Status: kube.ProwJobStatus{
				PodName: "wowowow",
				BuildID: "123",
				URL:     "https://gubernator.example.com/build/job/123",
			},
		},
	}
	fakeConfigAgent := fca{
		c: config.Config{
			ProwConfig: config.ProwConfig{
				Plank: config.Plank{
					JobURLPrefix: "https://gubernator.example.com/build/",
				},
			},
		},
	}
	fakeJa = jobs.NewJobAgent(kc, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA")}, &config.Agent{})
	fakeJa.Start()

	fakeGCSClient := fakeGCSServer.Client()

	sg := New(fakeJa, fakeConfigAgent, fakeGCSClient)

	result, err := sg.FetchArtifacts("prowjob/job/123", "", 500e6, []string{"build-log.txt"})
	if err != nil {
		t.Fatalf("Unexpected error grabbing pod log: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(result))
	}
	content, err := result[0].ReadAll()
	if err != nil {
		t.Fatalf("Unexpected error reading pod log: %v", err)
	}
	if string(content) != "clusterA" {
		t.Fatalf("Bad pod log content: %q (expected 'clusterA')", content)
	}
}
