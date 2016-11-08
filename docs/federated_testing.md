# Federated Testing

The quality of Kubernetes depends on exercising the software on a
variety of platforms.  The Kubernetes project welcomes contributions of
test results from organizations that execute e2e test jobs.  The
[24-hour test history
dashboard](http://storage.googleapis.com/kubernetes-test-history/static/index.html)
displays a summary of all results for the past day.
[TestGrid](https://k8s-testgrid.appspot.com/) shows the latest results.

We are actively working on improving this process, which means that this
document may not be kept exactly up-to-date. Feel free to file an issue against
this repo when you run into problems.


## Overview

To contribute test results for a platform, an organization executes the
e2e test cases and uploads the test results to a Google Cloud Storage
(GCS) bucket.  The test-infra repo contains scripts in `jenkins/` to run the
tests and to upload the results.

The various dashboards will read test results from these buckets. The remainder
of this document explains how to contribute test results.


### Run the e2e tests and upload results

Setup a periodic test job that will run at least once per day.  The test
job runs e2e tests and uploads the results.

1. Create a Google Cloud Storage bucket that will store the test
   results.  The bucket should have public read access.  The GCS URL
   must be stored in an environment variable named
   `JENKINS_GCS_LOGS_PATH`.  For example, Google's Jenkins server stores
   results to `gs://kubernetes-jenkins/logs/`.
2. Run `jenkins/dockerized-e2e-runner.sh` from the test-infra repo.
   There are several environment variables that affect the behavior of this
   script.  See [e2e-runner
   Environment Variables](#e2e-runner-environment-variables). This will run
   the tests from within a docker container. If this won't work for you, you
   can try running `jenkins/e2e-image/e2e-runner.sh`.
3. Run `JENKINS_BUILD_FINISHED={SUCCESS|UNSTABLE|FAILURE|ABORTED}
   hack/jenkins/upload-to-gcs.sh` from the kubernetes repo to upload the results
   to the GCS bucket.  If running from a Jenkins job, it is recommend to perform
   this step as a post-build step.  See
   `jenkins/job-configs/kubernetes-jenkins/global.yaml` for an example.


### Include results in test history

Collect results from federated test jobs by adding the Google Cloud Storage
bucket to [`buckets.yaml`](/buckets.yaml). This will also allow Gubernator to
track your jobs. To add buckets to the TestGrid, open an issue against this
repo.

### e2e-runner Environment Variables

The following environment variables should be set appropriately before
running the `e2e-runner.sh` script:

- WORKSPACE *
- JOB_NAME *
- BUILD_NUMBER *
- GINKGO_TEST_ARGS
- JENKINS_GCS_LOGS_PATH
- JENKINS_USE_EXISTING_BINARIES (optional. Set to "y" for locally built binaries.)
- E2E_TEST="true"

\* If using Jenkins to execute the e2e test job, then these environment
   variables may be set by Jenkins.  Otherwise, they must be set before
   executing `e2e-runner.sh`.  See above for how they affect how results
   are uploaded to a GCS bucket.

Test results are stored in
`gs://$JENKINS_GCS_LOGS_PATH/$JOB_NAME/$BUILD_NUMBER`. Every run should upload
`started.json`, `finished.json`, and `build-log.txt`, and can optionally upload
JUnit XML to the `artifacts/` directory.

If the platform should be setup or torn down by `e2e-runner.sh`, then
optionally set:

- E2E_UP="true"
- E2E_DOWN="true"

