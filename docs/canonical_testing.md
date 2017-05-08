# Canonical testing overview

We want to make it trivial for everyone to discover how to run tests, and to run
tests the same way.

In order to do this we will provide:

* A new standardized command to invoke any test job, which calls
  - `k8stest`
  - This knows how to map jobs into a job class.
  - This knows how to schedule a job locally, on docker or kubernetes.
* Standardize the command for each job class
  - `hack/e2e.go`, `hack/test-go.sh`, etc.
  - This command encapsulates everything needed to run a job, potentially with
    some customization passed in as an argument.
* Create a standardized image for each job class.
  - `kubernetes-e2e`, `test-go`, etc
  - This image encapsulates all the dependencies required to run the job at any
    given state of the repo and allows for the required customization.
* A standardized yaml files for each job that customizes the class.
  - `kubernetes-e2e-gce.yaml`, `kubernetes-gke-slow`, etc.
  - This defines how each job customizes the class. For example we run the same
    unit tests at different apis, and we run the same e2e flow with differnt
    ginkgo foci.

This will allow:

* There is a deterministic way to map a job to a class to commands in this
  class.
* Different people can confidently run the job the same way jenkins will do so.
* All jobs can run inside or outside a container.
* All jobs can run on or off a cluster.


## k8stest

The top level command allows


* A {job: (class, configuration)} mapping.
* A {set: [job]} mapping.
* Logic to map configuration to each class.
* Flags to pass in options
  - where and whether to upload results.
  - any required configuration to access a cluster and/or provider

Some basic examples:

```
k8stest test-go  # Run the unit tests
k8stest test-integration  # Run the integration tests
k8stest verify  # Run the verify tests
k8stest e2e-gce  # Run the e2e tests on gce
k8stest e2e-gke-serial  # Run the serial tests on gke
```

Customizing how a specific job runs:
```
k8stest test-go -- pkg/api pkg/kubelet # Run only the api and kubelet unit tests
k8stest test-integration -- Deadline  # Run only Deadline integration tests
k8stest verify -- godeps  # Run only the godep verification
k8stest e2e-gce -- --push --test  # Push to an existing cluster and run tests
k8stest e2e-gke-serial -- --skip=Daemon  # Skip the Daemon tests
k8stest e2e-gce-example -- --focus=rollover  # Only run the rollover tests
```

Customizing where things run:
```
k8stest --mode=docker  # Run tests inside a local container
k8stest --mode=k8s --config=provider-config.yaml # Schedule a pod to run tests
k8stest --mode=local --upload=upload-config.yaml # Upload results somewhere
```


## job class commands

Right now we have a bunch of useful scripts like `hack/e2e.go`,
`hack/verify-all.sh` and the like.

We will make these the official entry point that defines what happens for each
job class. This also defines the public interface between what users can call
and the raw `kubectl`, `go test`, etc commands issued during the job.

In other words we will expect developers (and automation) to either a) call
these commands directly or else make the raw calls that these scripts are
composed of.

Specifically accessing any intermediate scripts like `ginkgo-e2e.sh` will no
longer be supported and may atrophe over time without warning.

Many jobs typically follow a similar pattern. For example essentially every
`kubernetes-e2e-*` job issues the same `hack/e2e.go --build --up --test --down`
command with different extra arguments and environment variables. Therefore
these are encapsulated in the e2e class.


## job images

Each class of job will have a companion image that:

* contains all the dependencies required to complete the job class
* logic to deal with customization that varies per job:
  - job class customization lives in a well-known folder
* logic to deal with customization per user:
  - provider customization (project, creds, etc) lives in another well-known folder
  - upload customization (same but for uploads) lives in a third well-known folder

For example the e2e images will contain everything to run `hack/e2e.go` as well
as whatever else remains the same between different jobs in the class.

Each image will expect the customization to live in a particular location. The
image will

* process this customized information
* set the customized arguments/environment variables
* run the job class command included in the image.

## Yaml customization

The way each job will customize the class is through a yaml file. The yaml file
will specify things that the image needs to customize -- arguments and
environment variables.

When jobs run locally the top-level runner will duplicate the logic that
normally happens inside the container: it will know how to process the yaml file
to set the arguments and variables.

The code to do this inside or outside the container will be the same.


# Additional details

##  k8stest

```
k8stest [ARGS] [JOB ...] [-- JOB_ARGS]

JOB: a test job like kubernetes-build or kubernetes-e2e-gce or job set alias.
  unit: set of all unit test jobs like verify/build/etc
  e2e: set of all e2e jobs that run before commit
  blocking: set of all jobs that block the submit queue
  critical: set of all jobs that file issues on flakes.

  By default the unit and e2e jobs run.

JOB_ARGS: optionally args to send to every job.
  Use this to run a subset of tests for example.

ARGS: optional args that control how tests are run.
  --mode: controls where tests run
    --mode=local: runs tests locally outside a container (default)
    --mode=docker: runs tests locally inside a container
    --mode=k8s: run starts a k8s job to run each tests
  --config=CONFIG: configuration required to run suites, required for e2e tests
    defaults to the config section of ~/.k8stest if present
  --upload=UPLOAD_CONFIG: configuration to upload results, false by default
    defaults to the upload section of ~/.k8stest if present

  By default we do not build or upload.  and run locally.

CONFIG: a yaml file describing information necessary to deploy a cluster.
  projects available for use
  kubectl credentials
  provider credentials

UPLOAD\_CONFIG: a yaml file describing information necessary to transfer data.
  gsutil credentials
  prefix to builds
  prefix for results
```


## Yaml customization

This will have two keys: `arguments` and `variables`, each of which is a list of
items.

```
arguments:
- '--foo'
- '--eggs=spam'
- 'hello'
variables:
- 'X=1'
- 'B="foo bar"'
```
