## Overview

CI on GKE is a work-in-progress replacement for Jenkins. It comprises three
Kubernetes objects, with associated binaries under `cmd/`. The `hook` deployment
listens for GitHub webhooks. When a PR is opened or updated, it creates a job
to test it, called `test-pr`. The `test-pr` job downloads and merges the
source, uploads it to GCS, and reads the `.test.yml` file in the repository.
Based on that file, it starts `run-test` jobs that run the actual test
images.

## Usage

See the [Makefile](Makefile) for details. This is a work in progress. Once it
has settled down we will stop using the makefile for cluster management in
favor of static YAML files.

### Start a cluster

Edit the variables at the top of the `Makefile`. Your project will need to have
container engine enabled and enough quota for your nodes. You will need to know
the HMAC secret for your webhook and the OAuth token for your robot account.
Leave `DRY_RUN` set to `false` unless you want comments and statuses and the
like to appear on GitHub.

Now run `make create-cluster`. This will take a few minutes. It will create
a GKE cluster and the `hook` image, deployment, and service. Once it finishes,
it will wait for the external IP to show up and print out the webhook address.
Add that to your GitHub webhook.

### Update images

Bump the image version of whatever you are updating and run `make
update-cluster`.
