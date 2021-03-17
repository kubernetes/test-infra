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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	coreapi "k8s.io/api/core/v1"
	"k8s.io/test-infra/prow/gcsupload"
	"k8s.io/test-infra/prow/pod-utils/downwardapi"

	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tgconf "github.com/GoogleCloudPlatform/testgrid/pb/config"

	prowapi "k8s.io/test-infra/prow/apis/prowjobs/v1"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/io"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/spyglass/api"
	"k8s.io/test-infra/prow/spyglass/lenses"
	"k8s.io/test-infra/prow/spyglass/lenses/common"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	fakeJa        *jobs.JobAgent
	fakeGCSServer *fakestorage.Server
)

const (
	testSrc = "gs://test-bucket/logs/example-ci-run/403"
)

type fkc []prowapi.ProwJob

func (f fkc) List(ctx context.Context, pjs *prowapi.ProwJobList, _ ...ctrlruntimeclient.ListOption) error {
	pjs.Items = f
	return nil
}

type fpkc string

func (f fpkc) GetLogs(name, container string) ([]byte, error) {
	if name == "wowowow" || name == "powowow" {
		return []byte(fmt.Sprintf("%s.%s", f, container)), nil
	}
	return nil, fmt.Errorf("pod not found: %s", name)
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
		{
			BucketName: "test-bucket",
			Name:       "logs/symlink-party/123.txt",
			Content:    []byte(`gs://test-bucket/logs/the-actual-place/123`),
		},
		{
			BucketName: "multi-container-one-log",
			Name:       "logs/job/123/test-1-build-log.txt",
			Content:    []byte("this log exists in gcs!"),
		},
	})
	defer fakeGCSServer.Stop()
	kc := fkc{
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "job",
			},
			Status: prowapi.ProwJobStatus{
				PodName: "wowowow",
				BuildID: "123",
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent:   prowapi.KubernetesAgent,
				Job:     "jib",
				Cluster: "trusted",
			},
			Status: prowapi.ProwJobStatus{
				PodName: "powowow",
				BuildID: "123",
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "example-ci-run",
				PodSpec: &coreapi.PodSpec{
					Containers: []coreapi.Container{
						{
							Image: "tester",
						},
					},
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName: "wowowow",
				BuildID: "404",
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "multiple-container-job",
				PodSpec: &coreapi.PodSpec{
					Containers: []coreapi.Container{
						{
							Name: "test-1",
						},
						{
							Name: "test-2",
						},
					},
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName: "wowowow",
				BuildID: "123",
			},
		},
	}
	fakeJa = jobs.NewJobAgent(context.Background(), kc, false, true, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, fca{}.Config)
	fakeJa.Start()
	os.Exit(m.Run())
}

type dumpLens struct{}

func (dumpLens) Config() lenses.LensConfig {
	return lenses.LensConfig{
		Name:  "dump",
		Title: "Dump View",
	}
}

func (dumpLens) Header(artifacts []api.Artifact, resourceDir string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	return ""
}

func (dumpLens) Body(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
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

func (dumpLens) Callback(artifacts []api.Artifact, resourceDir string, data string, config json.RawMessage, spyglassConfig config.Spyglass) string {
	return ""
}

func TestViews(t *testing.T) {
	fakeGCSClient := fakeGCSServer.Client()
	testCases := []struct {
		name               string
		registeredViewers  []lenses.Lens
		lenses             []int
		expectedLensTitles []string
	}{
		{
			name:               "Spyglass basic test",
			registeredViewers:  []lenses.Lens{dumpLens{}},
			lenses:             []int{0},
			expectedLensTitles: []string{"Dump View"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, l := range tc.registeredViewers {
				lenses.RegisterLens(l)
			}
			c := fca{
				c: config.Config{
					ProwConfig: config.ProwConfig{
						Deck: config.Deck{
							Spyglass: config.Spyglass{
								Lenses: []config.LensFileConfig{
									{
										Lens: config.LensConfig{
											Name: "dump",
										},
									},
								},
							},
						},
					},
				},
			}
			sg := New(context.Background(), fakeJa, c.Config, io.NewGCSOpener(fakeGCSClient), false)
			_, ls := sg.Lenses(tc.lenses)
			for _, l := range ls {
				var found bool
				for _, title := range tc.expectedLensTitles {
					if title == l.Config().Title {
						found = true
					}
				}
				if !found {
					t.Errorf("lens title %s not found in expected titles.", l.Config().Title)
				}
			}
			for _, title := range tc.expectedLensTitles {
				var found bool
				for _, l := range ls {
					if title == l.Config().Title {
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
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Type: prowapi.PeriodicJob,
				Job:  "example-periodic-job",
				DecorationConfig: &prowapi.DecorationConfig{
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket: "chum-bucket",
					},
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "1111",
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Job:  "example-presubmit-job",
				DecorationConfig: &prowapi.DecorationConfig{
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket: "chum-bucket",
					},
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "2222",
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Job:  "undecorated-job",
			},
			Status: prowapi.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "1",
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Type:             prowapi.PresubmitJob,
				Job:              "missing-gcs-job",
				DecorationConfig: &prowapi.DecorationConfig{},
			},
			Status: prowapi.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "1",
			},
		},
	}
	fakeJa = jobs.NewJobAgent(context.Background(), kc, false, true, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, fca{}.Config)
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
			expJobPath: "gs/kubernetes-jenkins/logs/example-job-name",
		},
		{
			name:       "non-presubmit job in GCS without trailing /",
			src:        "gcs/kubernetes-jenkins/logs/example-job-name/123",
			expJobPath: "gs/kubernetes-jenkins/logs/example-job-name",
		},
		{
			name:       "presubmit job in GCS with trailing /",
			src:        "gcs/kubernetes-jenkins/pr-logs/pull/test-infra/0000/example-job-name/314159/",
			expJobPath: "gs/kubernetes-jenkins/pr-logs/directory/example-job-name",
		},
		{
			name:       "presubmit job in GCS without trailing /",
			src:        "gcs/kubernetes-jenkins/pr-logs/pull/test-infra/0000/example-job-name/314159",
			expJobPath: "gs/kubernetes-jenkins/pr-logs/directory/example-job-name",
		},
		{
			name:       "non-presubmit Prow job",
			src:        "prowjob/example-periodic-job/1111",
			expJobPath: "gs/chum-bucket/logs/example-periodic-job",
		},
		{
			name:       "Prow presubmit job",
			src:        "prowjob/example-presubmit-job/2222",
			expJobPath: "gs/chum-bucket/pr-logs/directory/example-presubmit-job",
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
		fakeOpener := io.NewGCSOpener(fakeGCSClient)
		fca := config.Agent{}
		sg := New(context.Background(), fakeJa, fca.Config, fakeOpener, false)
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

func TestProwJobName(t *testing.T) {
	kc := fkc{
		prowapi.ProwJob{
			ObjectMeta: metav1.ObjectMeta{Name: "flying-whales-1"},
			Spec: prowapi.ProwJobSpec{
				Type: prowapi.PeriodicJob,
				Job:  "example-periodic-job",
				DecorationConfig: &prowapi.DecorationConfig{
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket: "chum-bucket",
					},
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "1111",
			},
		},
		prowapi.ProwJob{
			ObjectMeta: metav1.ObjectMeta{Name: "flying-whales-2"},
			Spec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Job:  "example-presubmit-job",
				DecorationConfig: &prowapi.DecorationConfig{
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket: "chum-bucket",
					},
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "2222",
			},
		},
		prowapi.ProwJob{
			ObjectMeta: metav1.ObjectMeta{Name: "flying-whales-3"},
			Spec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Job:  "undecorated-job",
			},
			Status: prowapi.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "1",
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Type:             prowapi.PresubmitJob,
				Job:              "missing-name-job",
				DecorationConfig: &prowapi.DecorationConfig{},
			},
			Status: prowapi.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "1",
			},
		},
	}
	fakeJa = jobs.NewJobAgent(context.Background(), kc, false, true, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, fca{}.Config)
	fakeJa.Start()
	testCases := []struct {
		name       string
		src        string
		expJobPath string
		expError   bool
	}{
		{
			name:       "non-presubmit job in GCS without trailing /",
			src:        "gcs/kubernetes-jenkins/logs/example-periodic-job/1111/",
			expJobPath: "flying-whales-1",
		},
		{
			name:       "presubmit job in GCS with trailing /",
			src:        "gcs/kubernetes-jenkins/pr-logs/pull/test-infra/0000/example-presubmit-job/2222/",
			expJobPath: "flying-whales-2",
		},
		{
			name:       "non-presubmit Prow job",
			src:        "prowjob/example-periodic-job/1111",
			expJobPath: "flying-whales-1",
		},
		{
			name:       "Prow presubmit job",
			src:        "prowjob/example-presubmit-job/2222",
			expJobPath: "flying-whales-2",
		},
		{
			name:       "nonexistent job",
			src:        "prowjob/example-periodic-job/0000",
			expJobPath: "",
		},
		{
			name:       "job missing name",
			src:        "prowjob/missing-name-job/1",
			expJobPath: "",
		},
		{
			name:       "previously invalid key type is now valid but nonexistent",
			src:        "oh/my/glob/drama/bomb",
			expJobPath: "",
		},
		{
			name:     "invalid GCS path",
			src:      "gcs/kubernetes-jenkins/bad-path",
			expError: true,
		},
	}
	for _, tc := range testCases {
		fakeGCSClient := fakeGCSServer.Client()
		fakeOpener := io.NewGCSOpener(fakeGCSClient)
		fca := config.Agent{}
		sg := New(context.Background(), fakeJa, fca.Config, fakeOpener, false)
		jobPath, err := sg.ProwJobName(tc.src)
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

func TestRunPath(t *testing.T) {
	kc := fkc{
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Type: prowapi.PeriodicJob,
				Job:  "example-periodic-job",
				DecorationConfig: &prowapi.DecorationConfig{
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket: "chum-bucket",
					},
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "1111",
				URL:     "http://magic/view/gcs/chum-bucket/logs/example-periodic-job/1111",
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Job:  "example-presubmit-job",
				DecorationConfig: &prowapi.DecorationConfig{
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket: "chum-bucket",
					},
				},
				Refs: &prowapi.Refs{
					Org:  "some-org",
					Repo: "some-repo",
					Pulls: []prowapi.Pull{
						{
							Number: 42,
						},
					},
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "2222",
				URL:     "http://magic/view/gcs/chum-bucket/pr-logs/pull/some-org_some-repo/42/example-presubmit-job/2222",
			},
		},
	}
	fakeJa = jobs.NewJobAgent(context.Background(), kc, false, true, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, fca{}.Config)
	fakeJa.Start()
	testCases := []struct {
		name       string
		src        string
		expRunPath string
		expError   bool
	}{
		{
			name:       "non-presubmit job in GCS with trailing /",
			src:        "gcs/kubernetes-jenkins/logs/example-job-name/123/",
			expRunPath: "kubernetes-jenkins/logs/example-job-name/123",
		},
		{
			name:       "non-presubmit job in GCS without trailing /",
			src:        "gcs/kubernetes-jenkins/logs/example-job-name/123",
			expRunPath: "kubernetes-jenkins/logs/example-job-name/123",
		},
		{
			name:       "presubmit job in GCS with trailing /",
			src:        "gcs/kubernetes-jenkins/pr-logs/pull/test-infra/0000/example-job-name/314159/",
			expRunPath: "kubernetes-jenkins/pr-logs/pull/test-infra/0000/example-job-name/314159",
		},
		{
			name:       "presubmit job in GCS without trailing /",
			src:        "gcs/kubernetes-jenkins/pr-logs/pull/test-infra/0000/example-job-name/314159",
			expRunPath: "kubernetes-jenkins/pr-logs/pull/test-infra/0000/example-job-name/314159",
		},
		{
			name:       "non-presubmit Prow job",
			src:        "prowjob/example-periodic-job/1111",
			expRunPath: "chum-bucket/logs/example-periodic-job/1111",
		},
		{
			name:       "Prow presubmit job with full path",
			src:        "prowjob/example-presubmit-job/2222",
			expRunPath: "chum-bucket/pr-logs/pull/some-org_some-repo/42/example-presubmit-job/2222",
		},
		{
			name:     "nonexistent job",
			src:      "prowjob/example-periodic-job/0000",
			expError: true,
		},
		{
			name:       "previously invalid key type is now valid",
			src:        "oh/my/glob/drama/bomb",
			expRunPath: "my/glob/drama/bomb",
		},
		{
			name:     "nonsense string errors",
			src:      "this is not useful",
			expError: true,
		},
	}
	for _, tc := range testCases {
		fakeGCSClient := fakeGCSServer.Client()
		fakeOpener := io.NewGCSOpener(fakeGCSClient)
		fca := config.Agent{}
		fca.Set(&config.Config{
			ProwConfig: config.ProwConfig{
				Plank: config.Plank{
					JobURLPrefixConfig: map[string]string{"*": "http://magic/view/gcs/"},
				},
			},
		})
		sg := New(context.Background(), fakeJa, fca.Config, fakeOpener, false)
		jobPath, err := sg.RunPath(tc.src)
		if tc.expError && err == nil {
			t.Errorf("test %q: RunPath(%q) expected error, got  %q", tc.name, tc.src, jobPath)
			continue
		}
		if !tc.expError && err != nil {
			t.Errorf("test %q: RunPath(%q) returned unexpected error %v", tc.name, tc.src, err)
			continue
		}
		if jobPath != tc.expRunPath {
			t.Errorf("test %q: RunPath(%q) expected %q, got %q", tc.name, tc.src, tc.expRunPath, jobPath)
		}
	}
}

func TestRunToPR(t *testing.T) {
	kc := fkc{
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Type: prowapi.PeriodicJob,
				Job:  "example-periodic-job",
				DecorationConfig: &prowapi.DecorationConfig{
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket: "chum-bucket",
					},
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "1111",
				URL:     "http://magic/view/gcs/chum-bucket/logs/example-periodic-job/1111",
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Type: prowapi.PresubmitJob,
				Job:  "example-presubmit-job",
				DecorationConfig: &prowapi.DecorationConfig{
					GCSConfiguration: &prowapi.GCSConfiguration{
						Bucket: "chum-bucket",
					},
				},
				Refs: &prowapi.Refs{
					Org:  "some-org",
					Repo: "some-repo",
					Pulls: []prowapi.Pull{
						{
							Number: 42,
						},
					},
				},
			},
			Status: prowapi.ProwJobStatus{
				PodName: "flying-whales",
				BuildID: "2222",
			},
		},
	}
	fakeJa = jobs.NewJobAgent(context.Background(), kc, false, true, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, fca{}.Config)
	fakeJa.Start()
	testCases := []struct {
		name      string
		src       string
		expOrg    string
		expRepo   string
		expNumber int
		expError  bool
	}{
		{
			name:      "presubmit job in GCS with trailing /",
			src:       "gcs/kubernetes-jenkins/pr-logs/pull/Katharine_test-infra/1234/example-job-name/314159/",
			expOrg:    "Katharine",
			expRepo:   "test-infra",
			expNumber: 1234,
		},
		{
			name:      "presubmit job in GCS without trailing /",
			src:       "gcs/kubernetes-jenkins/pr-logs/pull/Katharine_test-infra/1234/example-job-name/314159",
			expOrg:    "Katharine",
			expRepo:   "test-infra",
			expNumber: 1234,
		},
		{
			name:      "presubmit job in GCS without org name",
			src:       "gcs/kubernetes-jenkins/pr-logs/pull/test-infra/2345/example-job-name/314159",
			expOrg:    "kubernetes",
			expRepo:   "test-infra",
			expNumber: 2345,
		},
		{
			name:      "presubmit job in GCS without org or repo name",
			src:       "gcs/kubernetes-jenkins/pr-logs/pull/3456/example-job-name/314159",
			expOrg:    "kubernetes",
			expRepo:   "kubernetes",
			expNumber: 3456,
		},
		{
			name:      "Prow presubmit job",
			src:       "prowjob/example-presubmit-job/2222",
			expOrg:    "some-org",
			expRepo:   "some-repo",
			expNumber: 42,
		},
		{
			name:     "Prow periodic job errors",
			src:      "prowjob/example-periodic-job/1111",
			expError: true,
		},
		{
			name:     "GCS periodic job errors",
			src:      "gcs/kuberneretes-jenkins/logs/example-periodic-job/1111",
			expError: true,
		},
		{
			name:     "GCS job with non-numeric PR number errors",
			src:      "gcs/kubernetes-jenkins/pr-logs/pull/asdf/example-job-name/314159",
			expError: true,
		},
		{
			name:     "GCS PR job in directory errors",
			src:      "gcs/kubernetes-jenkins/pr-logs/directory/example-job-name/314159",
			expError: true,
		},
		{
			name:     "Bad GCS key errors",
			src:      "gcs/this is just nonsense",
			expError: true,
		},
		{
			name:     "Longer bad GCS key errors",
			src:      "gcs/kubernetes-jenkins/pr-logs",
			expError: true,
		},
		{
			name:     "Nonsense string errors",
			src:      "friendship is magic",
			expError: true,
		},
	}
	for _, tc := range testCases {
		fakeGCSClient := fakeGCSServer.Client()
		fca := config.Agent{}
		fca.Set(&config.Config{
			ProwConfig: config.ProwConfig{
				Plank: config.Plank{
					DefaultDecorationConfigs: config.DefaultDecorationMapToSliceTesting(
						map[string]*prowapi.DecorationConfig{
							"*": {
								GCSConfiguration: &prowapi.GCSConfiguration{
									Bucket:       "kubernetes-jenkins",
									DefaultOrg:   "kubernetes",
									DefaultRepo:  "kubernetes",
									PathStrategy: "legacy",
								},
							},
						}),
				},
			},
		})
		sg := New(context.Background(), fakeJa, fca.Config, io.NewGCSOpener(fakeGCSClient), false)
		org, repo, num, err := sg.RunToPR(tc.src)
		if tc.expError && err == nil {
			t.Errorf("test %q: RunToPR(%q) expected error", tc.name, tc.src)
			continue
		}
		if !tc.expError && err != nil {
			t.Errorf("test %q: RunToPR(%q) returned unexpected error %v", tc.name, tc.src, err)
			continue
		}
		if org != tc.expOrg || repo != tc.expRepo || num != tc.expNumber {
			t.Errorf("test %q: RunToPR(%q) expected %s/%s#%d, got %s/%s#%d", tc.name, tc.src, tc.expOrg, tc.expRepo, tc.expNumber, org, repo, num)
		}
	}
}

func TestProwToGCS(t *testing.T) {
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
		kc := fkc{
			prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Job: "gubernator-job",
				},
				Status: prowapi.ProwJobStatus{
					URL:     "https://gubernator.example.com/build/some-bucket/gubernator-job/1111/",
					BuildID: "1111",
				},
			},
			prowapi.ProwJob{
				Spec: prowapi.ProwJobSpec{
					Job: "spyglass-job",
				},
				Status: prowapi.ProwJobStatus{
					URL:     "https://prow.example.com/view/gcs/some-bucket/spyglass-job/2222/",
					BuildID: "2222",
				},
			},
		}

		fakeGCSClient := fakeGCSServer.Client()
		fakeConfigAgent := fca{
			c: config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{
						JobURLPrefixConfig: map[string]string{"*": tc.configPrefix},
					},
				},
			},
		}
		fakeJa = jobs.NewJobAgent(context.Background(), kc, false, true, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, fakeConfigAgent.Config)
		fakeJa.Start()
		sg := New(context.Background(), fakeJa, fakeConfigAgent.Config, io.NewGCSOpener(fakeGCSClient), false)

		_, p, err := sg.prowToGCS(tc.key)
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

func TestGCSPathRoundTrip(t *testing.T) {
	testCases := []struct {
		name         string
		pathStrategy string
		defaultOrg   string
		defaultRepo  string
		org          string
		repo         string
	}{
		{
			name:         "simple explicit path",
			pathStrategy: "explicit",
			org:          "test-org",
			repo:         "test-repo",
		},
		{
			name:         "explicit path with underscores",
			pathStrategy: "explicit",
			org:          "test-org",
			repo:         "underscore_repo",
		},
		{
			name:         "'single' path with default repo",
			pathStrategy: "single",
			defaultOrg:   "default-org",
			defaultRepo:  "default-repo",
			org:          "default-org",
			repo:         "default-repo",
		},
		{
			name:         "'single' path with non-default repo",
			pathStrategy: "single",
			defaultOrg:   "default-org",
			defaultRepo:  "default-repo",
			org:          "default-org",
			repo:         "random-repo",
		},
		{
			name:         "'single' path with non-default org but default repo",
			pathStrategy: "single",
			defaultOrg:   "default-org",
			defaultRepo:  "default-repo",
			org:          "random-org",
			repo:         "default-repo",
		},
		{
			name:         "'single' path with non-default org and repo",
			pathStrategy: "single",
			defaultOrg:   "default-org",
			defaultRepo:  "default-repo",
			org:          "random-org",
			repo:         "random-repo",
		},
		{
			name:         "legacy path with default repo",
			pathStrategy: "legacy",
			defaultOrg:   "default-org",
			defaultRepo:  "default-repo",
			org:          "default-org",
			repo:         "default-repo",
		},
		{
			name:         "legacy path with non-default repo",
			pathStrategy: "legacy",
			defaultOrg:   "default-org",
			defaultRepo:  "default-repo",
			org:          "default-org",
			repo:         "random-repo",
		},
		{
			name:         "legacy path with non-default org but default repo",
			pathStrategy: "legacy",
			defaultOrg:   "default-org",
			defaultRepo:  "default-repo",
			org:          "random-org",
			repo:         "default-repo",
		},
		{
			name:         "legacy path with non-default org and repo",
			pathStrategy: "legacy",
			defaultOrg:   "default-org",
			defaultRepo:  "default-repo",
			org:          "random-org",
			repo:         "random-repo",
		},
		{
			name:         "legacy path with non-default org and repo with underscores",
			pathStrategy: "legacy",
			defaultOrg:   "default-org",
			defaultRepo:  "default-repo",
			org:          "random-org",
			repo:         "underscore_repo",
		},
	}

	for _, tc := range testCases {
		kc := fkc{}
		fakeConfigAgent := fca{
			c: config.Config{
				ProwConfig: config.ProwConfig{
					Plank: config.Plank{
						DefaultDecorationConfigs: config.DefaultDecorationMapToSliceTesting(
							map[string]*prowapi.DecorationConfig{
								"*": {
									GCSConfiguration: &prowapi.GCSConfiguration{
										DefaultOrg:  tc.defaultOrg,
										DefaultRepo: tc.defaultRepo,
									},
								},
							}),
					},
				},
			},
		}
		fakeJa = jobs.NewJobAgent(context.Background(), kc, false, true, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, fakeConfigAgent.Config)
		fakeJa.Start()

		fakeGCSClient := fakeGCSServer.Client()

		sg := New(context.Background(), fakeJa, fakeConfigAgent.Config, io.NewGCSOpener(fakeGCSClient), false)
		gcspath, _, _ := gcsupload.PathsForJob(
			&prowapi.GCSConfiguration{Bucket: "test-bucket", PathStrategy: tc.pathStrategy},
			&downwardapi.JobSpec{
				Job:     "test-job",
				BuildID: "1234",
				Type:    prowapi.PresubmitJob,
				Refs: &prowapi.Refs{
					Org: tc.org, Repo: tc.repo,
					Pulls: []prowapi.Pull{{Number: 42}},
				},
			}, "")
		fmt.Println(gcspath)
		org, repo, prnum, err := sg.RunToPR("gcs/test-bucket/" + gcspath)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
			continue
		}
		if org != tc.org || repo != tc.repo || prnum != 42 {
			t.Errorf("expected %s/%s#42, got %s/%s#%d", tc.org, tc.repo, org, repo, prnum)
		}
	}
}

func TestTestGridLink(t *testing.T) {
	testCases := []struct {
		name     string
		src      string
		expQuery string
		expError bool
	}{
		{
			name:     "non-presubmit job in GCS with trailing /",
			src:      "gcs/kubernetes-jenkins/logs/periodic-job/123/",
			expQuery: "some-dashboard#periodic",
		},
		{
			name:     "non-presubmit job in GCS without trailing /",
			src:      "gcs/kubernetes-jenkins/logs/periodic-job/123",
			expQuery: "some-dashboard#periodic",
		},
		{
			name:     "presubmit job in GCS",
			src:      "gcs/kubernetes-jenkins/pr-logs/pull/test-infra/0000/presubmit-job/314159/",
			expQuery: "some-dashboard#presubmit",
		},
		{
			name:     "non-presubmit Prow job",
			src:      "prowjob/periodic-job/1111",
			expQuery: "some-dashboard#periodic",
		},
		{
			name:     "presubmit Prow job",
			src:      "prowjob/presubmit-job/2222",
			expQuery: "some-dashboard#presubmit",
		},
		{
			name:     "nonexistent job",
			src:      "prowjob/nonexistent-job/0000",
			expError: true,
		},
		{
			name:     "invalid key type",
			src:      "oh/my/glob/drama/bomb",
			expError: true,
		},
		{
			name:     "nonsense string errors",
			src:      "this is not useful",
			expError: true,
		},
	}

	kc := fkc{}
	fakeJa = jobs.NewJobAgent(context.Background(), kc, false, true, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, fca{}.Config)
	fakeJa.Start()

	tg := TestGrid{c: &tgconf.Configuration{
		Dashboards: []*tgconf.Dashboard{
			{
				Name: "some-dashboard",
				DashboardTab: []*tgconf.DashboardTab{
					{
						Name:          "periodic",
						TestGroupName: "periodic-job",
					},
					{
						Name:          "presubmit",
						TestGroupName: "presubmit-job",
					},
					{
						Name:          "some-other-job",
						TestGroupName: "some-other-job",
					},
				},
			},
		},
	}}

	for _, tc := range testCases {
		fakeGCSClient := fakeGCSServer.Client()
		fca := config.Agent{}
		fca.Set(&config.Config{
			ProwConfig: config.ProwConfig{
				Deck: config.Deck{
					Spyglass: config.Spyglass{
						TestGridRoot: "https://testgrid.com/",
					},
				},
			},
		})
		sg := New(context.Background(), fakeJa, fca.Config, io.NewGCSOpener(fakeGCSClient), false)
		sg.testgrid = &tg
		link, err := sg.TestGridLink(tc.src)
		if tc.expError {
			if err == nil {
				t.Errorf("test %q: TestGridLink(%q) expected error, got  %q", tc.name, tc.src, link)
			}
			continue
		}
		if err != nil {
			t.Errorf("test %q: TestGridLink(%q) returned unexpected error %v", tc.name, tc.src, err)
			continue
		}
		if link != "https://testgrid.com/"+tc.expQuery {
			t.Errorf("test %q: TestGridLink(%q) expected %q, got %q", tc.name, tc.src, "https://testgrid.com/"+tc.expQuery, link)
		}
	}
}

func TestFetchArtifactsPodLog(t *testing.T) {
	kc := fkc{
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "job",
			},
			Status: prowapi.ProwJobStatus{
				PodName: "wowowow",
				BuildID: "123",
				URL:     "https://gubernator.example.com/build/job/123",
			},
		},
		prowapi.ProwJob{
			Spec: prowapi.ProwJobSpec{
				Agent: prowapi.KubernetesAgent,
				Job:   "multi-container-one-log",
			},
			Status: prowapi.ProwJobStatus{
				PodName: "wowowow",
				BuildID: "123",
				URL:     "https://gubernator.example.com/build/multi-container/123",
			},
		},
	}
	fakeConfigAgent := fca{
		c: config.Config{
			ProwConfig: config.ProwConfig{
				Plank: config.Plank{
					JobURLPrefixConfig: map[string]string{"*": "https://gubernator.example.com/build/"},
				},
			},
		},
	}
	fakeJa = jobs.NewJobAgent(context.Background(), kc, false, true, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, fakeConfigAgent.Config)
	fakeJa.Start()

	fakeGCSClient := fakeGCSServer.Client()

	sg := New(context.Background(), fakeJa, fakeConfigAgent.Config, io.NewGCSOpener(fakeGCSClient), false)
	testKeys := []string{
		"prowjob/job/123",
		"gcs/kubernetes-jenkins/logs/job/123/",
		"gcs/kubernetes-jenkins/logs/job/123",
	}

	for _, key := range testKeys {
		result, err := sg.FetchArtifacts(context.Background(), key, "", 500e6, []string{"build-log.txt"})
		if err != nil {
			t.Errorf("Unexpected error grabbing pod log for %s: %v", key, err)
			continue
		}
		if len(result) != 1 {
			t.Errorf("Expected 1 artifact for %s, got %d", key, len(result))
			continue
		}
		content, err := result[0].ReadAll()
		if err != nil {
			t.Errorf("Unexpected error reading pod log for %s: %v", key, err)
			continue
		}
		if string(content) != fmt.Sprintf("clusterA.%s", kube.TestContainerName) {
			t.Errorf("Bad pod log content for %s: %q (expected 'clusterA')", key, content)
		}
	}

	multiContainerOneLogKey := "gcs/multi-container-one-log/logs/job/123"

	testKeys = append(testKeys, multiContainerOneLogKey)

	for _, key := range testKeys {
		containers := []string{"test-1", "test-2"}
		result, err := sg.FetchArtifacts(context.Background(), key, "", 500e6, []string{fmt.Sprintf("%s-%s", containers[0], singleLogName), fmt.Sprintf("%s-%s", containers[1], singleLogName)})
		if err != nil {
			t.Errorf("Unexpected error grabbing pod log for %s: %v", key, err)
			continue
		}
		for i, art := range result {
			content, err := art.ReadAll()
			if err != nil {
				t.Errorf("Unexpected error reading pod log for %s: %v", key, err)
				continue
			}
			expected := fmt.Sprintf("clusterA.%s", containers[i])
			if key == multiContainerOneLogKey && containers[i] == "test-1" {
				expected = "this log exists in gcs!"
			}
			if string(content) != expected {
				t.Errorf("Bad pod log content for %s: %q (expected '%s')", key, content, expected)
			}
		}
	}
}

func TestKeyToJob(t *testing.T) {
	testCases := []struct {
		name      string
		path      string
		jobName   string
		buildID   string
		expectErr bool
	}{
		{
			name:    "GCS periodic path with trailing slash",
			path:    "gcs/kubernetes-jenkins/logs/periodic-kubernetes-bazel-test-1-14/40/",
			jobName: "periodic-kubernetes-bazel-test-1-14",
			buildID: "40",
		},
		{
			name:    "GCS periodic path without trailing slash",
			path:    "gcs/kubernetes-jenkins/logs/periodic-kubernetes-bazel-test-1-14/40",
			jobName: "periodic-kubernetes-bazel-test-1-14",
			buildID: "40",
		},
		{
			name:    "GCS PR path with trailing slash",
			path:    "gcs/kubernetes-jenkins/pr-logs/pull/test-infra/11573/pull-test-infra-bazel/25366/",
			jobName: "pull-test-infra-bazel",
			buildID: "25366",
		},
		{
			name:    "GCS PR path without trailing slash",
			path:    "gcs/kubernetes-jenkins/pr-logs/pull/test-infra/11573/pull-test-infra-bazel/25366",
			jobName: "pull-test-infra-bazel",
			buildID: "25366",
		},
		{
			name:    "Prowjob path with trailing slash",
			path:    "prowjob/pull-test-infra-bazel/25366/",
			jobName: "pull-test-infra-bazel",
			buildID: "25366",
		},
		{
			name:    "Prowjob path without trailing slash",
			path:    "prowjob/pull-test-infra-bazel/25366",
			jobName: "pull-test-infra-bazel",
			buildID: "25366",
		},
		{
			name:      "Path with only one component",
			path:      "nope",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		jobName, buildID, err := common.KeyToJob(tc.path)
		if err != nil {
			if !tc.expectErr {
				t.Errorf("%s: unexpected error %v", tc.name, err)
			}
			continue
		}
		if tc.expectErr {
			t.Errorf("%s: expected an error, but got result %s #%s", tc.name, jobName, buildID)
			continue
		}
		if jobName != tc.jobName {
			t.Errorf("%s: expected job name %q, but got %q", tc.name, tc.jobName, jobName)
			continue
		}
		if buildID != tc.buildID {
			t.Errorf("%s: expected build ID %q, but got %q", tc.name, tc.buildID, buildID)
		}
	}
}

func TestResolveSymlink(t *testing.T) {
	testCases := []struct {
		name      string
		path      string
		result    string
		expectErr bool
	}{
		{
			name:   "symlink without trailing slash is resolved",
			path:   "gcs/test-bucket/logs/symlink-party/123",
			result: "gs/test-bucket/logs/the-actual-place/123",
		},
		{
			name:   "symlink with trailing slash is resolved",
			path:   "gcs/test-bucket/logs/symlink-party/123/",
			result: "gs/test-bucket/logs/the-actual-place/123",
		},
		{
			name:   "non-symlink without trailing slash is unchanged",
			path:   "gcs/test-bucket/better-logs/42",
			result: "gs/test-bucket/better-logs/42",
		},
		{
			name:   "non-symlink with trailing slash drops the slash",
			path:   "gcs/test-bucket/better-logs/42/",
			result: "gs/test-bucket/better-logs/42",
		},
		{
			name:   "prowjob without trailing slash is unchanged",
			path:   "prowjob/better-logs/42",
			result: "prowjob/better-logs/42",
		},
		{
			name:   "prowjob with trailing slash drops the slash",
			path:   "prowjob/better-logs/42/",
			result: "prowjob/better-logs/42",
		},
		{
			name:      "unknown key type is an error",
			path:      "wtf/what-is-this/send-help",
			expectErr: true,
		},
		{
			name:      "insufficient path components are an error",
			path:      "gcs/hi",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		fakeConfigAgent := fca{}
		fakeJa = jobs.NewJobAgent(context.Background(), fkc{}, false, true, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, fakeConfigAgent.Config)
		fakeJa.Start()

		fakeGCSClient := fakeGCSServer.Client()

		sg := New(context.Background(), fakeJa, fakeConfigAgent.Config, io.NewGCSOpener(fakeGCSClient), false)

		result, err := sg.ResolveSymlink(tc.path)
		if err != nil {
			if !tc.expectErr {
				t.Errorf("test %q: unexpected error: %v", tc.name, err)
			}
			continue
		}
		if tc.expectErr {
			t.Errorf("test %q: expected an error, but got result %q", tc.name, result)
			continue
		}
		if result != tc.result {
			t.Errorf("test %q: expected %q, but got %q", tc.name, tc.result, result)
			continue
		}
	}
}

func TestExtraLinks(t *testing.T) {
	testCases := []struct {
		name      string
		content   string
		links     []ExtraLink
		expectErr bool
	}{
		{
			name:  "does nothing without error given no started.json",
			links: nil,
		},
		{
			name:      "errors given a malformed started.json",
			content:   "this isn't json",
			expectErr: true,
		},
		{
			name:    "does nothing given metadata with no links",
			content: `{"metadata": {"somethingThatIsntLinks": 23}}`,
			links:   nil,
		},
		{
			name:    "returns well-formed links",
			content: `{"metadata": {"links": {"ResultStore": {"url": "http://resultstore", "description": "The thing that isn't spyglass"}}}}`,
			links:   []ExtraLink{{Name: "ResultStore", URL: "http://resultstore", Description: "The thing that isn't spyglass"}},
		},
		{
			name:    "returns links without a description",
			content: `{"metadata": {"links": {"ResultStore": {"url": "http://resultstore"}}}}`,
			links:   []ExtraLink{{Name: "ResultStore", URL: "http://resultstore"}},
		},
		{
			name:    "skips links without a URL",
			content: `{"metadata": {"links": {"No Link": {"description": "bad link"}, "ResultStore": {"url": "http://resultstore"}}}}`,
			links:   []ExtraLink{{Name: "ResultStore", URL: "http://resultstore"}},
		},
		{
			name:    "skips links without a name",
			content: `{"metadata": {"links": {"": {"url": "http://resultstore"}}}}`,
			links:   []ExtraLink{},
		},
		{
			name:    "returns no links when links is empty",
			content: `{"metadata": {"links": {}}}`,
			links:   []ExtraLink{},
		},
		{
			name:    "returns multiple links",
			content: `{"metadata": {"links": {"A": {"url": "http://a", "description": "A!"}, "B": {"url": "http://b"}}}}`,
			links:   []ExtraLink{{Name: "A", URL: "http://a", Description: "A!"}, {Name: "B", URL: "http://b"}},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var objects []fakestorage.Object
			if tc.content != "" {
				objects = []fakestorage.Object{
					{
						BucketName: "test-bucket",
						Name:       "logs/some-job/42/started.json",
						Content:    []byte(tc.content),
					},
				}
			}
			gcsServer := fakestorage.NewServer(objects)
			defer gcsServer.Stop()

			gcsClient := gcsServer.Client()
			fakeConfigAgent := fca{}
			fakeJa = jobs.NewJobAgent(context.Background(), fkc{}, false, true, map[string]jobs.PodLogClient{kube.DefaultClusterAlias: fpkc("clusterA"), "trusted": fpkc("clusterB")}, fakeConfigAgent.Config)
			fakeJa.Start()
			sg := New(context.Background(), fakeJa, fakeConfigAgent.Config, io.NewGCSOpener(gcsClient), false)

			result, err := sg.ExtraLinks(context.Background(), "gcs/test-bucket/logs/some-job/42")
			if err != nil {
				if !tc.expectErr {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
			sort.Slice(tc.links, func(i, j int) bool { return tc.links[i].Name < tc.links[j].Name })
			if !reflect.DeepEqual(result, tc.links) {
				t.Fatalf("Expected links %#v, got %#v", tc.links, result)
			}
		})
	}
}
