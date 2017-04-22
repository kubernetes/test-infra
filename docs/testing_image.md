# Testing Job Image Overview

This document provides additional detail about
the images described in the [canonical testing] proposal.

The basic idea is that each testing job will map to one of a handful of job
classes: unit, integration, verify, e2e, node, etc.

Each class of test job will define an image. The image includes:

* all the necessary dependencies to complete the job.
  - Required packages.
  - Scripts/binaries that drive the testing.
  - Common arguments and environment variables
* logic to deal with parameters that vary between jobs of the same class.
  - Arguments and environment variables that make each job unique
* logic to deal with parameters that change between users.
  - Arguments and environment variables that are unique for each user
    (credentials, for example).
* logic to deal with parameters that configure what happens to results
  - Credentials/location to upload results.

This will help make sure running these test is easy and consistent.

## Common logic

Each job requires logic to perform some common behavior:

* Find credentials in a known location
* Understand which job name, commit and optionally PR is under test.
* Copy and extract a tarball of the source and/or binaries from a known prefix.
* Parse yaml files to find arguments and environment variables to inject.
* Guarantee completion within a specified duration.
* Copy results to a known prefix.

In order to address this we will:

* Expect the image to adhere to a particular contract in terms of folder
  structure:
  - `/incoming/run/run-metadata.yaml  # job name, tarball location, author, commit`
  - `/incoming/provider/provider-config.yaml  #  credentials, project name, etc`
  - `/incoming/export/export-config.yaml  # credentials, export prefix for copying results`
* Copy binaries into these images that find and respond to the state of these
  folders.

Note that `provider-config.yaml` may not be used for tests that do not need to
deploy/access a remote cluster.

## Logic common to each class

Each class will define an entry point. Every job based on this class will invoke
this entry point. The entry point will control things common between jobs.
For example all unit test jobs run `hack/test-go.sh` which always runs `go test`
and produces junit.xml results; and all e2e jobs call `hack/e2e.go` to deploy a
cluster and call `ginkgo e2e.test`, etc.

## Logic unique to each job

Each job will:
* use common logic configure additional arguments and variables
  - `/incoming/job/job-config.yaml  # Common job configuration (arguments/variables)`
  - All the incoming files common to all jobs.
* call the entrypoint
  - using common logic to timeout after a specified duration.
* use common logic to upload results if requested
* exit 0 when everything passes otherwise non-zero


# Initial classes

Initially we will define seven job classes (and an image for each):

1. `build` - build kubernetes
2. `verify` - run verification scripts
3. `unit` - run unit tests
4. `integration` - run integration tests
5. `e2e` - run e2e tests
6. `node-e2e` - run node e2e test
7. `kubemark` - run kubemark tests


## build image

This image will build kubernetes on one or more platforms using a
`hack/build` script. This is analogous to `kubernetes-build`.

This script will replace myriad other pieces involved in building:
* `hack/e2e-internal/build-release.sh`
* `hack/build-go.sh`
* `hack/build-cross.sh`
* `hack/build-ui.sh`
* `hack/jenkins/build.sh`
* `go run hack/e2e.go --build`

The functionality in these scripts will be migrated into `hack/build`.
We will update the scripts to first become wrappers for `hack/build`.
Then we will deprecate the scripts. Then we will delete them.


### build job-config

This will control:
* the subset of components to build: ui, server, client, tests, for exampl
* the platforms to build each component

This will default to a fast build.

## verify image

This image will run one or more verify scripts using the `hack/verify`. This
is analogous to `kubernetes-verify`.

The myriad of verify-* scripts in `hack` will move to `hack/verify-scripts/`.
We will copy `hack/verify-all.sh` to `hack/verify` and make it accept arguments
about which verifications to perform.
Then we will make `hack/verify-all.sh` a wrapper to the new script.
Then we will deprecate the old script. Then delete it.

We will also delete `hack/jenkins/verify.sh` and
`hack/jenkins/verify-dockerized.sh`


### verify job-config

This will control:

* the verify subchecks to run

The image will default to run all verify checks.


## unit image

This image will run unit tests using the `hack/test-go.sh` script. This is
analgous to `kubernetes-test-go`.

This script will replace the cacophony of similar scripts:
* `hack/jenkins/gotest-dockerized.sh`
* `hack/jenkins/gotest.sh`
* `hack/jenkins/test-dockerized.sh`
* `hack/test-go.sh`

Any necessary logic will move into `hack/test-go.sh`.
In particular the kubectl unit tests at `hack/test-cmd.sh` will
migrate to a format that produce junit results.


### unit job-config

This will control:

* the set of api versions to test
* whether to include coverage data
* how many tests to run in parallel
* toggle race testing

This will default to match what happens on PR jenkins.


## integration image

This image will run integration tests using the `hack/test-integration.sh`. This
script is analogous to a subset of `kubernetes-test-go`.

This will decouple the integration tests from `hack/test-go.sh`. Alternatively
we could combine all the test-\* images into a single unit image. However the
advantage of moving the integration tests into their own image is to ensure that
unit tests never depend on a real etcd server.


### integration job-config

This will control:

* level of concurrency
* log level
* api versions

This will default to match what happens on PR jenkins.


## e2e image

This image will run e2e tests that deploy and test clusters on various
platforms via `hack/e2e.go`. This is analogous to the `kubernetes-e2e-*` jobs.

This script will eventually eliminate any intermediate scripts:
* `hack/e2e-internal/*` in particular


### e2e job-config

This will control:

* platform
* concurrency
* min startup pods
* fail on resource leak toggle
* ginkgo focus
* ginkgo skip
* node configuration (number, shape, disk size
* any other options.

This will default to match what happens on PR jenkins.


## node\_e2e image

This image will run node e2e tests via a new `hack/node_e2e.go` script. This is
analogous to the `*-gce-e2e-ci` and `*-dockercanarybuild-ci` jobs.

This script will move the logic to run tests that currently lives in
[node-e2e.yaml] and into a new program that performs these actions instead.


### node\_e2e job-config


This will control:

* git basedir
* test command
* test path
* docker filepath

## kubemark image

This image will run kubemark tests via a new `hack/kubemark.go` script. This is
analogous to the `kubernetes-kubemark-*`jobs.

This new script will extract the kubemark logic from
`hack/jenkins/e2e-runner.sh`, this will involve moving the `hack/e2e-internal`
logic back into e2e.go, making it composable and then having `kubemark.go` use
the shared logic rather than embedding `if kubemark` commands into within the
e2e test scripts.

### kubemark job-config

This will control:

* platform
* kubelet count
* concurrency
* min startup pods
* fail on resource leak toggle
* ginkgo focus
* ginkgo skip
* node configuration (number, shape, disk size
* any other options.

This will default to match what happens on PR jenkins.

[canonical testing]: https://github.com/fejta/test-infra/blob/image/docs/canonical_testing.md
[node-e2e.yaml]: https://github.com/kubernetes/test-infra/blob/master/jenkins/job-configs/kubernetes-jenkins/node-e2e.yaml
