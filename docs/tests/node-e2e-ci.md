# Running Kubernetes node e2e Conformance tests on RHEL

Currently in Red Hat, we have two periodic Jenkins jobs running Kubernetes node e2e Conformance tests on RHEL.
The first job runs the tests over standard `Kubelet` binary.
The second one runs the same set of tests over containerized `Kubelet`.
Both jobs are run over VM instances provisioned in AWS.

The document describes the actions that were needed to implement the jobs.
There are three steps to consider:

1. [Running node e2e tests locally](#running-node-e2e-tests-locally)
2. [Uploading test results to GCS bucket](#uploading-test-results-to-gcs-bucket)
3. [Publishing test results in the TestGrid](#publishing-test-results-in-the-testgrid)

The `Kubernetes` git repository already provides most of the code needed to run the node e2e tests.
Thus, all the effort reduces to running `Makefile` with a set of relevant parameters.
With [#56250](https://github.com/kubernetes/kubernetes/pull/56250)
merged we are able to run the tests over containerized `Kubelet` as well.

Once all the tests are finished, the test results are expected to be published
into a GCS bucket. At the same time, the GCS bucket needs to be registered
in the `TestGrid` so the results can be shared with the upstream community
(and block a new release of `Kubernetes` in case the tests fail and are required not to fail).

General upstream documentation on adding a new e2e tests is available at
[contributing-test-results.md](../contributing-test-results.md).

## Running node e2e tests locally

### Standard Kubelet

It's enough to run the following command from the root `Kubernetes` repository
directory:

```sh
KUBELET_FLAGS="--cgroup-driver=systemd --cgroups-per-qos=true --cgroup-root=/"
make test-e2e-node TEST_ARGS="--kubelet-flags=\"${KUBELET_FLAGS}\"" \
    FOCUS="Conformance"
```

The command builds all the necessary binaries and runs the node e2e test suite.
The RHEL requires the ``--cgroup-driver=systemd`` flag to be set.

### Containerized Kubelet

In this variant there are two steps required to do:

1. build `hyperkube` docker image
2. tell the node e2e tests to run the containerized variant with the `hyperkube` image

**Build hyperkube docker image**

Before the docker image can be created, the `hyperkube` binary needs to be built.
Assuming the `GOPATH` is properly set and the required `golang` version installed,
it's just enough to run:

```sh
cd $GOPATH/src/k8s.io/kubernetes
make WHAT="cmd/hyperkube"
```

Based on your architecture and OS, the `hyperkube` binary can be written
under `_output/local/bin/linux/amd64/` directory.
Once written, you can build the docker image by running:

```sh
cd $GOPATH/src/k8s.io/kubernetes/cluster/images/hyperkube
export REGISTRY=registry.access.redhat.com
# Expected location of the hyperkube binary
export HYPERKUBE_BIN="_output/local/bin/linux/amd64/hyperkube"
# Either latest or the current git commit
IMAGE_TAG=$(git describe --abbrev=0)
make build VERSION=${IMAGE_TAG} ARCH=amd64 BASEIMAGE=rhel7
```

The docker image tag is set to reflect the current commit in the `Kubernetes`
repository. The `rhel7` image is used as the base docker image.
Once run, image with the `hyperkube-amd64` name is built.

**Running `Conformance` tests**

Once the `hyperkube` docker image is built, the node e2e tests over containerized
`Kubelet` can be run via:

```sh
IMAGE_TAG=$(git describe --abbrev=0)
HYPERKUBE_IMAGE="registry.access.redhat.com/hyperkube-amd64:${IMAGE_TAG}"
# --cgroups-per-qos=true no longer available
KUBELET_FLAGS="--cgroup-driver=systemd --cgroup-root=/"

# --kubelet-containerized and --hyperkube-image introduced
# by https://github.com/kubernetes/kubernetes/pull/56250
make test-e2e-node TEST_ARGS="--kubelet-containerized=true \
    --hyperkube-image=\"${HYPERKUBE_IMAGE}\" \
    --kubelet-flags=\"${KUBELET_FLAGS}\"" \
    FOCUS="Conformance"
```

## Uploading test results to GCS bucket

First step is to get a GCS bucket, either to create new or use existing one.
Content of the bucket must be made publicly available (see https://cloud.google.com/storage/docs/access-control/making-data-public).
For periodic jobs the expected GCS path is in the following form (see [gcs bucket layout](https://github.com/kubernetes/test-infra/blob/master/gubernator/README.md#gcs-bucket-layout) description):

```sh
gs://kubernetes-github-redhat/logs/${JOB_NAME}/${BUILD_NUMBER}/
```

The `TestGrid` then expects the following content of each build:

* started.json

  **Example**:
  ```json
  {
    "node": "ip-172-18-0-237.ec2.internal",
    "timestamp": 1511906201,
    "repos": {
      "k8s.io/kubernetes": "master"
    },
    "version": "v1.10.0-alpha.0.684+51033c4dec6e00",
    "repo-version": "v1.10.0-alpha.0.684+51033c4dec6e00"
  }
  ```

* finished.json

  **Example**:
  ```json
  {
    "timestamp": 1511907565,
    "version": "v1.10.0-alpha.0.684+51033c4dec6e00",
    "result": "SUCCESS",
    "passed": true,
    "job-version": "v1.10.0-alpha.0.684+51033c4dec6e00",
    "metadata": {
      "repo": "k8s.io/kubernetes",
      "repos": {
        "k8s.io/kubernetes": "master"
      },
      "repo-commit": "51033c4dec6e00cbbb550fcc09940efc54e54f79",
      "version": "v1.10.0-alpha.0.684+51033c4dec6e00",
      "job-version": "v1.10.0-alpha.0.684+51033c4dec6e00"
    }
  }
  ```

* build-log.txt
* artifacts

  Directory that provides additional information about a build. E.g.
  * junit files
  * logs of individual nodes
  * metadata

Official description of the individual files and their content is described by [job artifacts gcs layout](https://github.com/kubernetes/test-infra/blob/master/gubernator/README.md#job-artifact-gcs-layout). You can check a real example with more data at https://console.cloud.google.com/storage/browser/kubernetes-jenkins/logs/ci-cri-containerd-node-e2e/2600.

## Publishing test results in TestGrid

To have the [TestGrid](https://testgrid.k8s.io/) consume the new build results, one needs to extend the TestGrid
configuration file at https://github.com/kubernetes/test-infra/blob/master/testgrid/config/config.yaml.

The header of the file describes what needs to be done to add new build.
The current jobs have been added through https://github.com/kubernetes/test-infra/pull/5693 PR.

Once the PR is merged, one has to wait up to 30 minutes until the GCS bucket processing is run, the job results are processed and available in the TestGrid.

## Publishing test results in BigQuery

Add the bucket to the list of GCS buckets at [/kettle/buckets.yaml]. Results will be updated daily, and appear in the [/kettle/README.md] BigQuery tables.
