# Displaying Kubernetes Conformance results with Testgrid

_Note: this document was manually converted to markdown from [the original Google Doc](https://docs.google.com/document/d/1lGvP89_DdeNO84I86BVAU4qY3h2VCRll45tGrpyx90A/)._

_The current state is documented in [README.md](./README.md)_

## Overview

Kubernetes [conformance](https://github.com/cncf/k8s-conformance) testing is required by the CNCF for the “Certified Kubernetes” [program](https://www.cncf.io/certification/software-conformance/). For this, the requirement is to deploy the cncf/k8s-conformance config to a cluster and provide the JUnit artifact and test log. We aim to provide easy support for uploading and displaying conformance test results on Testgrid so that cloud providers can provide publicly discoverable conformance results.
This tool should be easy to use both manually for development and as part of automated infrastructure / CI, integrating well with the existing conformance tools. We will automate as much as possible while keeping things simple.

### Why Testgrid?

[The Kubernetes Testgrid](http://testgrid.k8s.io/) provides public dashboards well suited to tracking many test runs and individual test results. Testgrid is used heavily to display Kubernetes CI results, many of which are a superset of conformance testing.

## Background

Contributing test results has [an existing workflow](https://github.com/kubernetes/test-infra/blob/1c33865831040aa4fa775b0627abb90389daac9b/docs/contributing-test-results.md#contributing-test-results) covering the GCS storage bucket and Testgrid configuration. This is not easily automated and will be outside the scope of the new tooling. Creating / uploading test results in the right GCS layout however is usually handled by [bootstrap](https://github.com/kubernetes/test-infra/blob/1c33865831040aa4fa775b0627abb90389daac9b/jenkins/bootstrap.py) which is a bit heavy-weight and overly tied to our CI infrastructure, we can automate this with a much lighter and simpler tool focused on conformance results. 

The conformance repo testing config is based on [Sonobuoy](https://github.com/heptio/sonobuoy), both Sonobuoy and the conformance repo provide docs for obtaining results from a running cluster via [snapshot](https://github.com/heptio/sonobuoy/blob/master/docs/snapshot.md) (a tarball of various results) or [kubectl cp](https://github.com/cncf/k8s-conformance/blob/master/instructions.md#running) of the e2e.log and junit.xml directly. Additionally Heptio provides a web based sonobuoy result system [Heptio Scanner](https://scanner.heptio.com/), which provides a downloadable tarball containing e2e.log and junit.xml at:

[https://scanner.heptio.com/3f15e956994d70722e8e306b7bd4d13d/diagnostics/download/](https://scanner.heptio.com/3f15e956994d70722e8e306b7bd4d13d/diagnostics/download/)

Where "[3f15e956994d70722e8e306b7bd4d13d](https://scanner.heptio.com/3f15e956994d70722e8e306b7bd4d13d/diagnostics/download/)" is the UUID of the Scanner run at:  

[https://scanner.heptio.com/3f15e956994d70722e8e306b7bd4d13d/diagnostics/](https://scanner.heptio.com/3f15e956994d70722e8e306b7bd4d13d/diagnostics/)

We should easily be able to support direct dumping from clusters as well as obtaining or reading the result files from Heptio Scanner.

## Design

We will convert each run’s `junit_01.xml` and `e2e.log` to Testgrid results automatically by parsing timestamps from `e2e.log` to create minimal `started.json` and `finished.json` entries good enough to meet [the required job artifact GCS layout](https://github.com/kubernetes/test-infra/blob/1c33865831040aa4fa775b0627abb90389daac9b/gubernator/README.md#job-artifact-gcs-layout). `junit_01.xml` will be uploaded to the artifacts directory and `e2e.log` will be uploaded as `build-log.txt`.

**TBD**: Testgrid / Gubernator also expect a `"result": "SUCCESS or FAILURE, the result of the build"` field in `finished.json`. For the MVP we can use "zero failures in the junit results == SUCCESS" but we may want something more sophisticated in the future (?)

### Command Line / Example Usage

For each scenario the command line will look something like the following.

- Forwarding to testgrid from e2e.log and junit.xml already obtained by the user (EG dumped from the cluster or downloaded manually from Scanner, etc...):

`conformance2testgrid --bucket=gs://kubernetes-jenkins/logs/foo-job-name --junit=/path/to/junit_01.xml --log=/path/to/e2e.log`

- Forwarding to testgrid from a running cluster that has just run the tests (assumes kubectl in path and KUBECONFIG pointed at the cluster):

`conformance2testgrid --bucket=gs://kubernetes-jenkins/logs/foo-job-name --dump-from-cluster`

- Forwarding to testgrid from Heptio Scanner results (automatically downloaded by the tool):

`conformance2testgrid --bucket=gs://kubernetes-jenkins/logs/foo-job-name --scanner-url=https://scanner.heptio.com/3f15e956994d70722e8e306b7bd4d13d/diagnostics/`

`conformance2testgrid --bucket=gs://kubernetes-jenkins/logs/foo-job-name --scanner-uuid=3f15e956994d70722e8e306b7bd4d13d`

_Note: Scanner integration was dropped in the final design in order to focus on Continuous Integration results, however the tool can still be used to upload scanner results once they've been downloaded._

### Implementation Considerations

Dependencies should be kept minimal to ensure easy adoption. Tentatively these will be limited to the [gcloud command-line / sdk](https://cloud.google.com/sdk/downloads) for GCS upload / auth and optionally kubectl / working KUBECONFIG for automatically dumping results from live clusters. Since gcloud requires Python 2.7.x the tool may be implemented as an otherwise self-contained Python script.

Build IDs in the GCS upload path need to be Incremental for Testgrid; normally in CI we vend the next ID from [tot](https://github.com/kubernetes/test-infra/tree/master/prow/cmd/tot), but we don’t want end users to depend on a tot deployment or need to specify the ID manually so we while we will allow setting the buildID from the CLI we will default to the parsed start timestamp as the ID. This should be unique and incremental enough for each conformance job. This will not support Gubernators “recent builds” view, however.
