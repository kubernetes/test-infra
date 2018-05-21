# Display Conformance Tests with Testgrid

This directory contains tooling for displaying [kubernetes conformance test](https://github.com/cncf/k8s-conformance) results on [testgrid](../README.md).


## Using

1. First you will need to set up a publicly readable GCS bucket per [contributing test results](https://github.com/kubernetes/test-infra/blob/master/docs/contributing-test-results.md#contributing-test-results).

2. Make a PR to test-infra adding your bucket to the testgrid config (again see [contributing test results](https://github.com/kubernetes/test-infra/blob/master/docs/contributing-test-results.md#contributing-test-results)).

3. Run the conformance tests, you can do this by:

 - [following the official guide](https://github.com/cncf/k8s-conformance/blob/master/instructions.md#running)

 - or using [kubetest](https://github.com/kubernetes/test-infra/tree/master/kubetest):
   - cd to a Kubernetes source tree for the release you wish to test
   - run `make quick-release` to build the test binaries
   - make sure `kubectl` / `$KUBECONFIG` is authed to your cluster
   - run kubetest with:
    ```sh
    export KUBERNETES_CONFORMANCE_TEST=y
    kubetest --provider=skeleton \
             --test --test_args="--ginkgo.focus=\[Conformance\]" \ 
             --dump=./_artifacts | tee ./e2e.log
    ```

 - or using [Heptio Scanner](https://scanner.heptio.com/)

4. Obtain the log file and JUnit xml output.

 - With the above kubetest command you can find the log file and JUnit at `./e2e.log` and `./_artifacts/junit_01.xml` respectively.

 - For [Sonobuouy](https://github.com/heptio/sonobuoy) run locally you can obtain a "snapshot" with the official instructions [when run locally](https://github.com/heptio/sonobuoy#download-and-run). You can then get the e2e log and junit from the snapshot (see the [plugins section](https://github.com/heptio/sonobuoy/blob/master/docs/snapshot.md#plugins) of the snapshot documentation)

 - For Sonobuoy run via Heptio Scanner you can click "download test report" and extract the log and junit files.

5. Authenticate [the gcloud sdk](https://cloud.google.com/sdk/downloads) / [gsutil](https://cloud.google.com/storage/docs/gsutil) to your GCS bucket

6. run `upload_e2e.py --junit=<path to junit_01.xml> --log=<path to log file> --bucket=gs://your-bucket`. Optionally you can supply the following
  * --year=YYYY, otherwise the current year on the host is assumed when parsing timestamps
  * --metadata='dictionary of additional key-value pairs that can be displayed to the user'. E.g: --metadata='{"testrun": "Run after master upgrade"}'.For more details please see [metadata for finished.json](https://github.com/kubernetes/test-infra/tree/master/gubernator#job-artifact-gcs-layout) and custom [column headers in TestGrid](https://github.com/kubernetes/test-infra/blob/master/testgrid/README.md#column-headers).

