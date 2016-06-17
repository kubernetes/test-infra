# Federated Testing

The quality of Kubernetes depends on exercising the software on a
variety of platforms.  The Kubernetes project welcomes contributions of
test results from organizations that execute e2e test jobs.  The
[Kubernetes test history
dashboard](http://storage.googleapis.com/kubernetes-test-history/static/index.html)
displays the latest results from all federated test jobs.


## Overview

To contribute test results for a platform, an organization executes the
e2e test cases and uploads the test results to a Google Cloud Storage
(GCS) bucket.  The Kubernetes project contains scripts in `hack/` to run
the tests and to upload the results.

The test history dashboard is generated periodically by scripts in this
repository that read test results from all of the GCS buckets.  The test
history shows test results from jobs run during the previous 24 hours.
A test job should execute at least once every 24 hours.

The remainder of this document explains how to contribute test results.


### Run the e2e tests and upload results

Setup a periodic test job that will run at least once per day.  The test
job runs e2e tests and uploads the results.

1. Create a Google Cloud Storage bucket that will store the test
   results.  The bucket should have public read access.  The GCS URL
   must be stored in an environment variable named
   JENKINS_GCS_LOGS_PATH.  For example, Google's Jenkins server stores
   results to `gs://kubernetes-jenkins/logs/`.
2. Run `hack/jenkins/e2e-runner.sh`.  There are several environment
   variables that affect the behavior of this script.  See [e2e-runner
   Environment Variables](#e2e-runner-environment-variables).
3. Run `JENKINS_BUILD_FINISHED={SUCCESS|UNSTABLE|FAILURE|ABORTED}
   hack/jenkins/upload-to-gcs.sh` to upload the results to the GCS
   bucket.  If running from a Jenkins job, it is recommend to perform
   this step as a post-build step.  See
   `jenkins/job-configs/global.yaml` for an example.


### Include results in test history

Collect results from federated test jobs by adding the Google Cloud Storage
bucket to [`buckets.yaml`](/buckets.yaml).

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
`gs://$JENKINS_GCS_LOGS_PATH/$JOB_NAME/$BUILD_NUMBER`.

If the platform should be setup or torn down by `e2e-runner.sh`, then
optionally set:

- E2E_UP="true"
- E2E_DOWN="true"

