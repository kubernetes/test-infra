# Display Conformance Tests with Testgrid

This directory contains tooling for uploading [Kubernetes Conformance test](https://github.com/cncf/k8s-conformance) results for display / monitoring on [TestGrid](../README.md), a tool used heavily by [the core kubernetes project](https://github.com/kubernetes/kubernetes) to monitor test results, particularly as part of the release process.

Federated conformance test results are hosted on the TestGrid [conformance dashboards](https://testgrid.k8s.io/conformance-all), including the "all" dashboard, and specific sub-dashboards, see [the TestGrid README](../README.md#dashboards) for details on dashboards. Generally we are aiming to have a dashboard here for each provider E.g. "[conformance-cloud-provider-openstack](https://testgrid.k8s.io/conformance-cloud-provider-openstack)" as well as [a cross-vendor dashboard](https://testgrid.k8s.io/conformance-all) to track project wide conformance.

All Kubernetes cluster providers are invited to post results from their conformance test jobs and results from reliable continuous integration against the release branches may even be used as a signal by the Kubernetes release team in the [release-blocking dashboards](https://testgrid.k8s.io/sig-release-master-blocking).

The release team has caught actual conformance test regressions using these dashboards just in the first month or so of setting up GCE / OpenStack conformance on TestGrid, and had them fixed before the Kubernetes 1.11 release.

For the original design doc and further details on the motivation please see [design.md](./design.md).

## Usage Guide

1. First you will need to set up a publicly readable GCS bucket per [contributing test results](https://github.com/kubernetes/test-infra/blob/master/docs/contributing-test-results.md#contributing-test-results) to host your jobs' results.
If you cannot or do not want to set up a GCS bucket and only wish to post conformance test results, please file an issue in https://github.com/kubernetes/k8s.io for a staging bucket, referencing https://github.com/kubernetes/k8s.io/pull/501.

2. Make a PR to test-infra adding your bucket to the TestGrid config (again see [contributing test results](https://github.com/kubernetes/test-infra/blob/master/docs/contributing-test-results.md#contributing-test-results)).

- See The following PR from setting up the initial OpenStack bucket: [#7670](https://github.com/kubernetes/test-infra/pull/7670)

3. Setup a job in your CI system to run the conformance tests. To use [`upload_e2e.py`](./upload_e2e.py) the job environment must have `python` (v3.X) and `gcloud` / `gsutil` commands. For the gcloud CLI see [Installing the Google Cloud SDK](https://cloud.google.com/sdk/downloads).

This job will need to:
   - `a)` setup a cluster from the kubernetes release / branch you want to test
   - `b)` run the conformance tests
   - `c)` obtain the JUnit .xml results and ginkgo (e2e test runner) log output
   - `d)` upload the results to the GCS bucket

Setting up the test cluster in `a)` is provider specific and not currently covered here.

For running the conformance tests and obtaining the result files (`b)` and `c)`) you have the following options:

 - follow [the official conformance testing guide's instructions](https://github.com/cncf/k8s-conformance/blob/master/instructions.md#running) to run and obtain the result files

 - or use [kubetest](https://github.com/kubernetes/test-infra/tree/master/kubetest):
   - cd to a Kubernetes source tree (git clone) for the release you wish to test, using something like:
   ```sh
   git clone https://github.com/kubernetes/kubernetes.git && cd kubernetes && git checkout release-1.11
   ```
   - run `make all WHAT="test/e2e/e2e.test vendor/github.com/onsi/ginkgo/ginkgo cmd/kubectl"` to build the test binaries
   - make sure `kubectl` / `$KUBECONFIG` is authed to your cluster
   - run [kubetest](https://github.com/kubernetes/test-infra/tree/master/kubetest) with:
    ```sh
    export KUBERNETES_CONFORMANCE_TEST=y
    kubetest --provider=skeleton \
             --test \
             --test_args="--ginkgo.focus=\[Conformance\]" \
             --dump=./_artifacts | tee ./e2e.log
    ```
   - You can then find the log file and JUnit at `./e2e.log` and `./_artifacts/junit_01.xml` respectively.

 - or use the [Sonobuouy CLI](https://github.com/heptio/sonobuoy#using-the-cli) to run the tests and then obtain a "snapshot" with the official instructions [when run locally](https://github.com/heptio/sonobuoy#download-and-run). You can then get the e2e log and JUnit from the snapshot (see the [plugins section](https://github.com/heptio/sonobuoy/blob/master/site/docs/master/snapshot.md#plugins) of the snapshot documentation)

For uploading the results (`d)`) you can use the tooling provided here (or build your own mimicking it), to use `upload_e2e.py` provide the following required flags:
- `--junit` -- The path to the JUnit result file(s): `--junit=/path/to/junit/result/file/junit_01.xml`
  - note that this flag accepts [glob patterns](https://en.wikipedia.org/wiki/Glob_(programming)), E.g. `--junit=./artifacts/junit_*.xml`

- `--log` -- The path to the ginkgo log file / test output:
 `--log=/path/to/e2e.log`

- `--bucket` -- The upload prefix, which should include the GCS bucket as well as the job name, E.g. `gs://k8s-conformance-openstack/periodic-logs/ci-cloud-provider-openstack-acceptance-test-e2e-conformance-stable-branch-v1.11/` like: `--bucket=gs://your-bucket/your-job`

You can optionally also provide:
- `--key-file` -- A Google Cloud [service account](https://cloud.google.com/iam/docs/service-accounts) keyfile, used to automatically authenticate to GCS. Otherwise you will need to authenticate with [`gcloud auth`](https://cloud.google.com/sdk/gcloud/reference/auth/) in some other part of your CI to use this tool.
Specify like:
`--key-file=/path/to-key-file.json`
  - If you are using a GKE EngProd provided bucket, we've provided you with this file, otherwise see [Create and Manage Service Accounts](https://cloud.google.com/iam/docs/managing-service-accounts), [Create and manage Service Account Keys](https://cloud.google.com/iam/docs/creating-managing-service-account-keys), and [Cloud Storage IAM Roles](https://cloud.google.com/storage/docs/access-control/iam-roles) for docs on setting up your own service account with upload access to the bucket and creating a credentials file for it.

- `--year` -- The year in which the logfile was produced, otherwise the current year on the host machine is assumed when parsing timestamps for the job's start / finish time. E.g. `--year=2018`

- `--metadata` -- A JSON dict of metadata key-value pairs that can be displayed in custom TestGrid column headers. E.g. `--metadata='{"version": 52e0b2617ffec85d467f96de34d47e9bb407f880"}'`.
  - For more details please see [metadata for finished.json](https://github.com/kubernetes/test-infra/tree/master/gubernator#job-artifact-gcs-layout) and custom [column headers in TestGrid](https://github.com/kubernetes/test-infra/blob/master/testgrid/README.md#column-headers).

