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

//Package spyglasstests contains tests for spyglass
package spyglasstests

import (
	"fmt"
	"os"
	"path"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/sirupsen/logrus"
	"k8s.io/test-infra/prow/config"
	"k8s.io/test-infra/prow/deck/jobs"
	"k8s.io/test-infra/prow/kube"
	"k8s.io/test-infra/prow/spyglass"
)

var (
	fakeGCSBucket    *storage.BucketHandle
	testAf           *spyglass.GCSArtifactFetcher
	fakeJa           *jobs.JobAgent
	fakeGCSJobSource *spyglass.GCSJobSource
	buildLogName     string
	longLogName      string
	startedName      string
	finishedName     string
	buildLogKey      string
	longLogKey       string
	startedKey       string
	finishedKey      string
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
	fakeGCSJobSource = spyglass.NewGCSJobSource(testSrc)
	testBucketName := fakeGCSJobSource.BucketName()
	buildLogName = "build-log.txt"
	startedName = "started.json"
	finishedName = "finished.json"
	longLogName = "long-log.txt"
	buildLogKey = path.Join(fakeGCSJobSource.JobPath(), buildLogName)
	startedKey = path.Join(fakeGCSJobSource.JobPath(), startedName)
	finishedKey = path.Join(fakeGCSJobSource.JobPath(), finishedName)
	longLogKey = path.Join(fakeGCSJobSource.JobPath(), longLogName)
	logrus.Info("Bucket keys: ", buildLogKey, finishedKey, startedKey, longLogKey)
	fakeGCSServer := fakestorage.NewServer([]fakestorage.Object{
		{
			BucketName: testBucketName,
			Name:       buildLogKey,
			Content:    []byte("Oh wow\nlogs\nthis is\ncrazy"),
		},
		{
			BucketName: testBucketName,
			Name:       longLogKey,
			Content:    longLog,
		},
		{
			BucketName: testBucketName,
			Name:       startedKey,
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
			Name:       finishedKey,
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
	testAf = &spyglass.GCSArtifactFetcher{
		Client: fakeGCSClient,
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

var (
	longLog = []byte(`Cloning into 'test-infra'...
I0709 19:00:47.085] Args: --job=pull-kubernetes-e2e-gce-100-performance --service-account=/etc/service-account/service-account.json --upload=gs://kubernetes-jenkins/logs --root=/go/src --repo=k8s.io/kubernetes=master:f70410959d2801320b528e287f2fcf764f261335,65986:3dfdfc16a26b77f014ca6507b7822b11d90f95a0 --repo=k8s.io/release --upload=gs://kubernetes-jenkins/pr-logs --timeout=120
I0709 19:00:47.085] Bootstrap pull-kubernetes-e2e-gce-100-performance...
I0709 19:00:47.088] Builder: gke-prow-default-pool-3c8994a8-hjfj
I0709 19:00:47.089] Image: gcr.io/k8s-testimages/kubekins-e2e:v20180706-944ac61f8-master
I0709 19:00:47.089] Gubernator results at https://k8s-gubernator.appspot.com/build/kubernetes-jenkins/pr-logs/pull/65986/pull-kubernetes-e2e-gce-100-performance/12540
I0709 19:00:47.089] Call:  gcloud auth activate-service-account --key-file=/etc/service-account/service-account.json
W0709 19:00:47.935] Activated service account credentials for: [pr-kubekins@kubernetes-jenkins-pull.iam.gserviceaccount.com]
I0709 19:00:48.277] process 18 exited with code 0 after 0.0m
I0709 19:00:48.278] Call:  gcloud config get-value account
I0709 19:00:48.899] process 33 exited with code 0 after 0.0m
I0709 19:00:48.900] Will upload results to gs://kubernetes-jenkins/pr-logs using pr-kubekins@kubernetes-jenkins-pull.iam.gserviceaccount.com
I0709 19:00:48.900] Call:  kubectl get -oyaml pods/4f84a034-83aa-11e8-9f54-0a580a6c0315
I0709 19:00:49.326] process 48 exited with code 0 after 0.0m
I0709 19:00:49.328] Call:  gsutil -q -h Content-Type:text/plain cp /tmp/gsutil_di85rG gs://kubernetes-jenkins/pr-logs/pull/65986/pull-kubernetes-e2e-gce-100-performance/12540/artifacts/prow_podspec.yaml
W0709 19:00:51.454] WARNING 0709 19:00:51.453440 multiprocess_file_storage.py] Credentials file could not be loaded, will ignore and overwrite.
W0709 19:00:51.454] WARNING 0709 19:00:51.453727 multiprocess_file_storage.py] Credentials file could not be loaded, will ignore and overwrite.
W0709 19:00:51.457] WARNING 0709 19:00:51.457360 multiprocess_file_storage.py] Credentials file could not be loaded, will ignore and overwrite.
W0709 19:00:51.457] WARNING 0709 19:00:51.457502 multiprocess_file_storage.py] Credentials file could not be loaded, will ignore and overwrite.
W0709 19:00:51.567] WARNING 0709 19:00:51.567300 multiprocess_file_storage.py] Credentials file could not be loaded, will ignore and overwrite.
I0709 19:00:52.076] process 61 exited with code 0 after 0.0m
I0709 19:00:52.077] Root: /go/src
I0709 19:00:52.077] cd to /go/src
I0709 19:00:52.077] Checkout: /go/src/k8s.io/kubernetes master:f70410959d2801320b528e287f2fcf764f261335,65986:3dfdfc16a26b77f014ca6507b7822b11d90f95a0 to /go/src/k8s.io/kubernetes
I0709 19:00:52.077] Call:  git init k8s.io/kubernetes
I0709 19:00:52.083] Initialized empty Git repository in /go/src/k8s.io/kubernetes/.git/
I0709 19:00:52.083] process 242 exited with code 0 after 0.0m
I0709 19:00:52.083] Call:  git config --local user.name 'K8S Bootstrap'
I0709 19:00:52.087] process 243 exited with code 0 after 0.0m
I0709 19:00:52.087] Call:  git config --local user.email k8s_bootstrap@localhost
I0709 19:00:52.090] process 244 exited with code 0 after 0.0m
I0709 19:00:52.090] Call:  git fetch --quiet --tags https://github.com/kubernetes/kubernetes master +refs/pull/65986/head:refs/pr/65986
I0709 19:01:44.561] process 245 exited with code 0 after 0.9m
I0709 19:01:44.562] Call:  git checkout -B test f70410959d2801320b528e287f2fcf764f261335
W0709 19:01:46.756] Switched to a new branch 'test'
I0709 19:01:46.760] process 257 exited with code 0 after 0.0m
I0709 19:01:46.760] Call:  git show -s --format=format:%ct HEAD
I0709 19:01:46.766] process 258 exited with code 0 after 0.0m
I0709 19:01:46.766] Call:  git merge --no-ff -m 'Merge +refs/pull/65986/head:refs/pr/65986' 3dfdfc16a26b77f014ca6507b7822b11d90f95a0
I0709 19:01:47.358] Merge made by the 'recursive' strategy.
I0709 19:01:47.361]  hack/verify-generated-files.sh | 10 ++++++++++
I0709 19:01:47.361]  1 file changed, 10 insertions(+)
I0709 19:01:47.364] process 259 exited with code 0 after 0.0m
I0709 19:01:47.364] Checkout: /go/src/k8s.io/release master to /go/src/k8s.io/release
I0709 19:01:47.364] Call:  git init k8s.io/release
I0709 19:01:47.386] Initialized empty Git repository in /go/src/k8s.io/release/.git/
I0709 19:01:47.386] process 261 exited with code 0 after 0.0m
I0709 19:01:47.387] Call:  git config --local user.name 'K8S Bootstrap'
I0709 19:01:47.398] process 262 exited with code 0 after 0.0m
I0709 19:01:47.399] Call:  git config --local user.email k8s_bootstrap@localhost
I0709 19:01:47.403] process 263 exited with code 0 after 0.0m
I0709 19:01:47.403] Call:  git fetch --quiet --tags https://github.com/kubernetes/release master
I0709 19:01:48.017] process 264 exited with code 0 after 0.0m
I0709 19:01:48.018] Call:  git checkout -B test FETCH_HEAD
W0709 19:01:48.093] Switched to a new branch 'test'
I0709 19:01:48.093] process 276 exited with code 0 after 0.0m
I0709 19:01:48.094] Call:  git show -s --format=format:%ct HEAD
I0709 19:01:48.098] process 277 exited with code 0 after 0.0m
I0709 19:01:48.098] Configure environment...
I0709 19:01:48.098] Call:  git show -s --format=format:%ct HEAD
I0709 19:01:48.102] process 278 exited with code 0 after 0.0m
I0709 19:01:48.103] Call:  gcloud auth activate-service-account --key-file=/etc/service-account/service-account.json
W0709 19:01:48.811] Activated service account credentials for: [pr-kubekins@kubernetes-jenkins-pull.iam.gserviceaccount.com]
I0709 19:01:49.153] process 279 exited with code 0 after 0.0m
I0709 19:01:49.153] Call:  gcloud config get-value account
I0709 19:01:49.707] process 294 exited with code 0 after 0.0m
I0709 19:01:49.707] Will upload results to gs://kubernetes-jenkins/pr-logs using pr-kubekins@kubernetes-jenkins-pull.iam.gserviceaccount.com
I0709 19:01:49.707] Call:  bash -c '
set -o errexit
set -o nounset
export KUBE_ROOT=.
source hack/lib/version.sh
kube::version::get_version_vars
echo $KUBE_GIT_VERSION
'
I0709 19:01:50.070] process 309 exited with code 0 after 0.0m
I0709 19:01:50.071] Start 12540 at v1.12.0-alpha.0.2026+14bc168e243b4e...
I0709 19:01:50.072] Call:  gsutil -q -h Content-Type:application/json cp /tmp/gsutil__Ca1aM gs://kubernetes-jenkins/pr-logs/pull/65986/pull-kubernetes-e2e-gce-100-performance/12540/started.json
I0709 19:01:57.493] process 342 exited with code 0 after 0.1m
I0709 19:01:57.494] Call:  gsutil -q -h Content-Type:text/plain -h 'x-goog-meta-link: gs://kubernetes-jenkins/pr-logs/pull/65986/pull-kubernetes-e2e-gce-100-performance/12540' cp /tmp/gsutil_ZSisuk gs://kubernetes-jenkins/pr-logs/directory/pull-kubernetes-e2e-gce-100-performance/12540.txt
I0709 19:01:59.855] process 523 exited with code 0 after 0.0m
I0709 19:01:59.869] Call:  /workspace/./test-infra/jenkins/../scenarios/kubernetes_e2e.py --build=bazel --cluster= --env-file=jobs/env/ci-kubernetes-e2e-scalability-common.env --env-file=jobs/env/ci-kubernetes-e2e-gci-gce-scalability.env --extract=local --gcp-nodes=100 --gcp-project=k8s-presubmit-scale --gcp-zone=us-east1-b --provider=gce --stage=gs://kubernetes-release-pull/ci/pull-kubernetes-e2e-gce-100-performance --tear-down-previous '--test_args=--ginkgo.focus=\[Feature:Performance\] --minStartupPods=8 --gather-resource-usage=true --gather-metrics-at-teardown=true' --timeout=100m --use-logexporter
W0709 19:01:59.912] starts with local mode
W0709 19:01:59.913] Environment:
W0709 19:01:59.913] BAZEL_REMOTE_CACHE_ENABLED=false
W0709 19:01:59.913] BAZEL_VERSION=0.14.0
W0709 19:01:59.913] BOOTSTRAP_MIGRATION=yes
W0709 19:01:59.913] BOSKOS_METRICS_PORT=tcp://10.63.249.148:80
W0709 19:01:59.913] BOSKOS_METRICS_PORT_80_TCP=tcp://10.63.249.148:80
W0709 19:01:59.913] BOSKOS_METRICS_PORT_80_TCP_ADDR=10.63.249.148
W0709 19:01:59.913] BOSKOS_METRICS_PORT_80_TCP_PORT=80
W0709 19:01:59.914] BOSKOS_METRICS_PORT_80_TCP_PROTO=tcp
W0709 19:01:59.914] BOSKOS_METRICS_SERVICE_HOST=10.63.249.148
W0709 19:01:59.914] BOSKOS_METRICS_SERVICE_PORT=80
W0709 19:01:59.914] BOSKOS_METRICS_SERVICE_PORT_DEFAULT=80
W0709 19:01:59.914] BOSKOS_PORT=tcp://10.63.250.132:80
W0709 19:01:59.914] BOSKOS_PORT_80_TCP=tcp://10.63.250.132:80
W0709 19:01:59.914] BOSKOS_PORT_80_TCP_ADDR=10.63.250.132
W0709 19:01:59.914] BOSKOS_PORT_80_TCP_PORT=80
W0709 19:01:59.914] BOSKOS_PORT_80_TCP_PROTO=tcp
W0709 19:01:59.914] BOSKOS_SERVICE_HOST=10.63.250.132
W0709 19:01:59.914] BOSKOS_SERVICE_PORT=80
W0709 19:01:59.915] BOSKOS_SERVICE_PORT_DEFAULT=80
W0709 19:01:59.915] BUILD_ID=12540
W0709 19:01:59.915] BUILD_NUMBER=12540
W0709 19:01:59.915] CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK=true
W0709 19:01:59.915] CLOUDSDK_CONFIG=/go/src/k8s.io/kubernetes/.config/gcloud
W0709 19:01:59.915] CLOUDSDK_CORE_DISABLE_PROMPTS=1
W0709 19:01:59.915] CLOUDSDK_EXPERIMENTAL_FAST_COMPONENT_UPDATE=false
W0709 19:01:59.915] CONTROLLER_MANAGER_TEST_ARGS=--profiling --kube-api-qps=100 --kube-api-burst=100
W0709 19:01:59.915] CREATE_CUSTOM_NETWORK=true
W0709 19:01:59.915] DOCKER_IN_DOCKER_ENABLED=false
W0709 19:01:59.915] ETCD_EXTRA_ARGS=--enable-pprof
W0709 19:01:59.916] GCS_ARTIFACTS_DIR=gs://kubernetes-jenkins/pr-logs/pull/65986/pull-kubernetes-e2e-gce-100-performance/12540/artifacts
W0709 19:01:59.916] GOOGLE_APPLICATION_CREDENTIALS=/etc/service-account/service-account.json
W0709 19:01:59.916] GOPATH=/go
W0709 19:01:59.916] GO_TARBALL=go1.10.2.linux-amd64.tar.gz
W0709 19:01:59.916] HOME=/workspace
W0709 19:01:59.916] HOSTNAME=4f84a034-83aa-11e8-9f54-0a580a6c0315
W0709 19:01:59.916] IMAGE=gcr.io/k8s-testimages/kubekins-e2e:v20180706-944ac61f8-master
W0709 19:01:59.916] INSTANCE_PREFIX=e2e-65986-95a39
W0709 19:01:59.916] JENKINS_AWS_SSH_PRIVATE_KEY_FILE=/root/.ssh/kube_aws_rsa
W0709 19:01:59.916] JENKINS_AWS_SSH_PUBLIC_KEY_FILE=/root/.ssh/kube_aws_rsa.pub
W0709 19:01:59.917] JENKINS_GCE_SSH_PRIVATE_KEY_FILE=/workspace/.ssh/google_compute_engine
W0709 19:01:59.917] JENKINS_GCE_SSH_PUBLIC_KEY_FILE=/workspace/.ssh/google_compute_engine.pub
W0709 19:01:59.917] JOB_NAME=pull-kubernetes-e2e-gce-100-performance
W0709 19:01:59.917] JOB_SPEC={"type":"presubmit","job":"pull-kubernetes-e2e-gce-100-performance","buildid":"12540","prowjobid":"4f84a034-83aa-11e8-9f54-0a580a6c0315","refs":{"org":"kubernetes","repo":"kubernetes","base_ref":"master","base_sha":"f70410959d2801320b528e287f2fcf764f261335","pulls":[{"number":65986,"author":"ixdy","sha":"3dfdfc16a26b77f014ca6507b7822b11d90f95a0"}]}}
W0709 19:01:59.917] JOB_TYPE=presubmit
W0709 19:01:59.917] KUBELET_TEST_ARGS=--enable-debugging-handlers
W0709 19:01:59.917] KUBEPROXY_TEST_ARGS=--profiling
W0709 19:01:59.918] KUBERNETES_PORT=tcp://10.63.240.1:443
W0709 19:01:59.918] KUBERNETES_PORT_443_TCP=tcp://10.63.240.1:443
W0709 19:01:59.918] KUBERNETES_PORT_443_TCP_ADDR=10.63.240.1
W0709 19:01:59.918] KUBERNETES_PORT_443_TCP_PORT=443
W0709 19:01:59.918] KUBERNETES_PORT_443_TCP_PROTO=tcp
W0709 19:01:59.918] KUBERNETES_SERVICE_HOST=10.63.240.1
W0709 19:01:59.918] KUBERNETES_SERVICE_PORT=443
W0709 19:01:59.918] KUBERNETES_SERVICE_PORT_HTTPS=443
W0709 19:01:59.918] KUBETEST_IN_DOCKER=true
W0709 19:01:59.918] KUBETEST_MANUAL_DUMP=y
W0709 19:01:59.918] KUBE_AWS_INSTANCE_PREFIX=e2e-65986-95a39
W0709 19:01:59.918] KUBE_GCE_ENABLE_IP_ALIASES=true
W0709 19:01:59.919] KUBE_GCE_INSTANCE_PREFIX=e2e-65986-95a39
W0709 19:01:59.919] LOGROTATE_MAX_SIZE=5G
W0709 19:01:59.919] NODE_DISK_SIZE=50GB
W0709 19:01:59.919] NODE_NAME=gke-prow-default-pool-3c8994a8-hjfj
W0709 19:01:59.919] NODE_SIZE=n1-standard-1
W0709 19:01:59.919] PATH=/go/bin:/usr/local/go/bin:/google-cloud-sdk/bin:/workspace:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
W0709 19:01:59.919] POD_NAME=4f84a034-83aa-11e8-9f54-0a580a6c0315
W0709 19:01:59.919] PREPULL_E2E_IMAGES=false
W0709 19:01:59.919] PROW_JOB_ID=4f84a034-83aa-11e8-9f54-0a580a6c0315
W0709 19:01:59.919] PULL_BASE_REF=master
W0709 19:01:59.919] PULL_BASE_SHA=f70410959d2801320b528e287f2fcf764f261335
W0709 19:01:59.920] PULL_NUMBER=65986
W0709 19:01:59.920] PULL_PULL_SHA=3dfdfc16a26b77f014ca6507b7822b11d90f95a0
W0709 19:01:59.920] PULL_REFS=master:f70410959d2801320b528e287f2fcf764f261335,65986:3dfdfc16a26b77f014ca6507b7822b11d90f95a0
W0709 19:01:59.920] PWD=/workspace
W0709 19:01:59.920] REGISTER_MASTER=true
W0709 19:01:59.920] REPO_NAME=kubernetes
W0709 19:01:59.920] REPO_OWNER=kubernetes
W0709 19:01:59.920] SCHEDULER_TEST_ARGS=--profiling --kube-api-qps=100 --kube-api-burst=100
W0709 19:01:59.920] SHLVL=1
W0709 19:01:59.920] SOURCE_DATE_EPOCH=1531158428
W0709 19:01:59.920] TERM=xterm
W0709 19:01:59.921] TEST_CLUSTER_DELETE_COLLECTION_WORKERS=--delete-collection-workers=16
W0709 19:01:59.921] TEST_CLUSTER_LOG_LEVEL=--v=2
W0709 19:01:59.921] TEST_CLUSTER_RESYNC_PERIOD=--min-resync-period=12h
W0709 19:01:59.921] TEST_TMPDIR=/bazel-scratch/.cache/bazel
W0709 19:01:59.921] USER=prow
W0709 19:01:59.921] WORKSPACE=/workspace
W0709 19:01:59.921] _=./test-infra/jenkins/bootstrap.py
W0709 19:01:59.922] Run: ('kubetest', '--dump=/workspace/_artifacts', '--gcp-service-account=/etc/service-account/service-account.json', '--build=bazel', '--stage=gs://kubernetes-release-pull/ci/pull-kubernetes-e2e-gce-100-performance', '--up', '--down', '--test', '--provider=gce', '--cluster=e2e-65986-95a39', '--gcp-network=e2e-65986-95a39', '--extract=local', '--gcp-nodes=100', '--gcp-project=k8s-presubmit-scale', '--gcp-zone=us-east1-b', '--test_args=--ginkgo.focus=\\[Feature:Performance\\] --minStartupPods=8 --gather-resource-usage=true --gather-metrics-at-teardown=true', '--timeout=100m', '--logexporter-gcs-path=gs://kubernetes-jenkins/pr-logs/pull/65986/pull-kubernetes-e2e-gce-100-performance/12540/artifacts')
W0709 19:02:00.003] 2018/07/09 19:02:00 main.go:322: Limiting testing to 1h40m0s
W0709 19:02:00.003] 2018/07/09 19:02:00 util.go:132: Please use kubetest --gcp-node-size=n1-standard-1 (instead of deprecated NODE_SIZE=n1-standard-1)
W0709 19:02:00.004] 2018/07/09 19:02:00 process.go:153: Running: gcloud config set project k8s-presubmit-scale
W0709 19:02:00.520] Updated property [core/project].
W0709 19:02:00.589] 2018/07/09 19:02:00 process.go:155: Step 'gcloud config set project k8s-presubmit-scale' finished in 585.85277ms
W0709 19:02:00.589] 2018/07/09 19:02:00 process.go:153: Running: gcloud auth activate-service-account --key-file=/etc/service-account/service-account.json
W0709 19:02:01.303] Activated service account credentials for: [pr-kubekins@kubernetes-jenkins-pull.iam.gserviceaccount.com]
W0709 19:02:01.358] 2018/07/09 19:02:01 process.go:155: Step 'gcloud auth activate-service-account --key-file=/etc/service-account/service-account.json' finished in 769.076264ms
W0709 19:02:01.359] 2018/07/09 19:02:01 main.go:814: Checking existing of GCP ssh keys...
W0709 19:02:01.359] 2018/07/09 19:02:01 main.go:824: Checking presence of public key in k8s-presubmit-scale
W0709 19:02:01.359] 2018/07/09 19:02:01 process.go:153: Running: gcloud compute --project=k8s-presubmit-scale project-info describe
W0709 19:02:02.346] 2018/07/09 19:02:02 process.go:155: Step 'gcloud compute --project=k8s-presubmit-scale project-info describe' finished in 987.218795ms
W0709 19:02:02.347] 2018/07/09 19:02:02 process.go:153: Running: make -C /go/src/k8s.io/kubernetes bazel-release
W0709 19:02:02.403] $TEST_TMPDIR defined: output root default is '/bazel-scratch/.cache/bazel' and max_idle_secs default is '15'.
W0709 19:02:02.406] Extracting Bazel installation...
I0709 19:02:02.507] make: Entering directory '/go/src/k8s.io/kubernetes'
W0709 19:02:13.613] Starting local Bazel server and connecting to it...
W0709 19:02:15.244] ..............
W0709 19:02:15.552] Loading: 
W0709 19:02:15.555] Loading: 0 packages loaded
W0709 19:02:16.558] Loading: 0 packages loaded
W0709 19:02:18.531] Loading: 0 packages loaded
W0709 19:02:19.559] Loading: 0 packages loaded
W0709 19:02:20.559] Analyzing: target //build/release-tars:release-tars (2 packages loaded)
W0709 19:02:24.538] Analyzing: target //build/release-tars:release-tars (2 packages loaded)
W0709 19:02:25.931] Analyzing: target //build/release-tars:release-tars (39 packages loaded)
W0709 19:02:27.510] Analyzing: target //build/release-tars:release-tars (172 packages loaded)
W0709 19:02:27.879] INFO: SHA256 (https://github.com/google/containerregistry/archive/v0.0.19.tar.gz) = aeae61d6f89d920f5743f7e8d0dbe902757c39a637a2747e27b67a2c1345a195
W0709 19:02:29.324] Analyzing: target //build/release-tars:release-tars (1009 packages loaded)
W0709 19:02:31.429] Analyzing: target //build/release-tars:release-tars (1013 packages loaded)
W0709 19:02:35.901] Analyzing: target //build/release-tars:release-tars (1013 packages loaded)
W0709 19:02:38.984] Analyzing: target //build/release-tars:release-tars (1023 packages loaded)
W0709 19:02:40.155] INFO: SHA256 (https://codeload.github.com/google/protobuf/zip/106ffc04be1abf3ff3399f54ccf149815b287dd9) = 4d127e1c3608803bc845cc4b3c8c42a6a16263b6ddcb932b53049e7ec8101398
W0709 19:02:42.551] Analyzing: target //build/release-tars:release-tars (1524 packages loaded)
W0709 19:02:46.746] Analyzing: target //build/release-tars:release-tars (1973 packages loaded)
W0709 19:02:49.369] INFO: Analysed target //build/release-tars:release-tars (2053 packages loaded).
W0709 19:02:49.373] INFO: Found 1 target...
W0709 19:02:49.532] [0 / 15] [-----] BazelWorkspaceStatusAction stable-status.txt
W0709 19:02:54.779] [11 / 62] Compiling external/com_google_protobuf/src/google/protobuf/compiler/java/java_lazy_message_field.cc [for host]; 2s linux-sandbox ... (8 actions running)
W0709 19:03:01.539] [28 / 62] Compiling external/com_google_protobuf/src/google/protobuf/compiler/java/java_service.cc [for host]; 3s linux-sandbox ... (8 actions, 7 running)
W0709 19:03:08.550] [47 / 78] Compiling external/com_google_protobuf/src/google/protobuf/generated_message_reflection.cc [for host]; 5s linux-sandbox ... (7 actions running)
W0709 19:03:16.920] [64 / 95] Compiling external/com_google_protobuf/src/google/protobuf/descriptor.cc [for host]; 9s linux-sandbox ... (7 actions running)
W0709 19:03:26.626] [81 / 112] Compiling external/com_google_protobuf/src/google/protobuf/compiler/js/js_generator.cc [for host]; 7s linux-sandbox ... (7 actions running)
W0709 19:03:37.878] [108 / 139] Compiling external/com_google_protobuf/src/google/protobuf/compiler/cpp/cpp_message.cc [for host]; 8s linux-sandbox ... (7 actions running)
W0709 19:03:50.791] [144 / 179] Compiling external/com_google_protobuf/src/google/protobuf/compiler/cpp/cpp_map_field.cc [for host]; 2s linux-sandbox ... (7 actions running)
W0709 19:04:05.514] [179 / 210] Compiling external/com_google_protobuf/src/google/protobuf/compiler/cpp/cpp_helpers.cc [for host]; 3s linux-sandbox ... (7 actions running)
W0709 19:04:22.480] [225 / 728] GoLink external/io_bazel_rules_go/go/tools/builders/linux_amd64_stripped/stdlib [for host]; 2s linux-sandbox ... (3 actions, 2 running)
W0709 19:04:43.639] [330 / 4,586] GoStdlib external/io_bazel_rules_go/linux_amd64_pure_stripped/stdlib~/pkg; 20s linux-sandbox ... (3 actions running)
W0709 19:05:05.876] [526 / 4,586] GoCompile vendor/gopkg.in/yaml.v2/linux_amd64_stripped/go_default_library~/k8s.io/kubernetes/vendor/gopkg.in/yaml.v2.a [for host]; 1s linux-sandbox ... (8 actions running)
W0709 19:05:31.451] [1,116 / 4,586] GoCompile external/com_github_golang_protobuf/proto/linux_amd64_stripped/go_default_library~/github.com/golang/protobuf/proto.a; 1s linux-sandbox ... (8 actions running)
W0709 19:06:00.863] [1,588 / 4,586] GoCompile staging/src/k8s.io/client-go/kubernetes/typed/authorization/v1/fake/linux_amd64_stripped/go_default_library~/k8s.io/kubernetes/vendor/k8s.io/client-go/kubernetes/typed/authorization/v1/fake.a; 0s linux-sandbox ... (8 actions running)
W0709 19:06:34.696] [2,038 / 4,586] GoCompile vendor/github.com/vmware/govmomi/vim25/types/linux_amd64_stripped/go_default_library~/k8s.io/kubernetes/vendor/github.com/vmware/govmomi/vim25/types.a; 15s linux-sandbox ... (8 actions running)`)
	longLogLines = strings.Split(string(longLog), "\n")
)
