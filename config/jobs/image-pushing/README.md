# Image pushing jobs

This directory contains jobs that run in the trusted cluster and kick off GCB
jobs that then push images to staging GCR repos. These jobs are the recommended
way to regularly publish images to staging.

## Getting started

You'll need a staging GCR. If you don't have one,
[instructions are over here][gcr instructions]. Once you have one, there are two
components two getting set up:

* A `cloudbuild.yaml` file in your repo, customised to build your images in
  whatever way works for you
* A cookie-cutter prow job config in this directory. These are almost the same
  for all builds, and variance should be avoided wherever possible.

## cloudbuild.yaml

The contents of `cloudbuild.yaml` depends on how your repo produces images.
The [official documentation][gcb documentation] discusses these in the general
case. If your image can be built using `go build` or Bazel and pushed using
`docker push`, that advice should be sufficient.

### Custom substitutions

We add two [custom substitutions][substitution docs] to your GCB builds:
`_GIT_TAG` and `_PULL_BASE_REF`.

`_GIT_TAG` will contain a tag of the form `vYYYYMMDD-hash`, `vYYYYMMDD-tag`, or
`vYYYYMMDD-tag-n-ghash`, depending on the git tags on your repo. We recommend
using `$_GIT_TAG` to tag your images.

`_PULL_BASE_REF` will contain the base ref that was pushed to - for instance,
`master` or `release-0.2` for a PR merge, or `v0.2` for a tag. You can use this
for logic in your build process (like deciding whether to update `latest`) if
desired.

### Simple build example

If your build just involves `go build` and `docker build`, you can use a
`cloudbuild.yaml` that looks something like this (assuming you use go modules):

```yaml
# See https://cloud.google.com/cloud-build/docs/build-config

# this must be specified in seconds. If omitted, defaults to 600s (10 mins)
timeout: 1200s
steps:
  - name: golang:1.13
    args: ['go', 'build', '-o', 'somebin', '.']
    env:
    - CGO_ENABLED=0
    - GOOS=linux
    - GOARCH=amd64
  - name: gcr.io/cloud-builders/docker
    args:
    - build
    - --tag=gcr.io/$PROJECT_ID/some-image:$_GIT_TAG
    - --tag=gcr.io/$PROJECT_ID/some-image:latest
    - .
    # default cloudbuild has HOME=/builder/home and docker buildx is in /root/.docker/cli-plugins/docker-buildx
    # set the home to /root explicitly to if using docker buildx
    # - HOME=/root
substitutions:
  _GIT_TAG: '12345'
  _PULL_BASE_REF: 'master'
# this prevents errors if you don't use both _GIT_TAG and _PULL_BASE_REF,
# or any new substitutions added in the future.
options:
  substitution_option: ALLOW_LOOSE
# this will push these images, or cause the build to fail if they weren't built.
images:
  - 'gcr.io/$PROJECT_ID/some-image:$_GIT_TAG'
  - 'gcr.io/$PROJECT_ID/some-image:latest'
```

### Makefile build example

If your build process is driven by a Makefile or similar, you can use GCB to
invoke that. We provide the [`gcr.io/k8s-testimages/gcb-docker-gcloud` image][gcb-docker-gcloud],
which contains components that are likely to be useful for your builds. A sample
`cloudbuild.yaml` using `make` to build and push might look like this:

```yaml
# See https://cloud.google.com/cloud-build/docs/build-config

# this must be specified in seconds. If omitted, defaults to 600s (10 mins)
timeout: 1200s
# this prevents errors if you don't use both _GIT_TAG and _PULL_BASE_REF,
# or any new substitutions added in the future.
options:
  substitution_option: ALLOW_LOOSE
steps:
  - name: 'gcr.io/k8s-testimages/gcb-docker-gcloud:v20190906-745fed4'
    entrypoint: make
    env:
    - DOCKER_CLI_EXPERIMENTAL=enabled
    - TAG=$_GIT_TAG
    - BASE_REF=$_PULL_BASE_REF
    args:
    - release-staging
substitutions:
  # _GIT_TAG will be filled with a git-based tag for the image, of the form vYYYYMMDD-hash, and
  # can be used as a substitution
  _GIT_TAG: '12345'
  # _PULL_BASE_REF will contain the ref that was pushed to to trigger this build -
  # a branch like 'master' or 'release-0.2', or a tag like 'v0.2'.
  _PULL_BASE_REF: 'master'
```

## Prow config template

These jobs run in the trusted cluster. As such, we will not accept variants that
run arbitrary code inside the prow job, or jobs that substantially deviate from
this template. One day, we may automate this away.

If you aren't sure, feel free to ask a [reviewer](./OWNERS) to do this part
for you. 

Prow config should be in a file named after your staging GCR project, and should
be based on this template:

```yaml
postsubmits:
  # This is the github repo we'll build from. This block needs to be repeated
  # for each repo.
  kubernetes-sigs/some-repo-name:
    # The name should be changed to match the repo name above
    - name: post-some-repo-name-push-images
      cluster: k8s-infra-prow-build-trusted
      annotations:
        # This is the name of some testgrid dashboard to report to.
        # If this is the first one for your sig, you may need to create one
        testgrid-dashboards: sig-something-image-pushes
      decorate: true
      # this causes the job to only run on the master branch. Remove it if your
      # job makes sense on every branch (unless it's setting a `latest` tag it
      # probably does).
      branches:
        - ^master$
      spec:
        serviceAccountName: gcb-builder
        containers:
          - image: gcr.io/k8s-testimages/image-builder:v20190906-d5d7ce3
            command:
              - /run.sh
            args:
              # this is the project GCB will run in, which is the same as the GCR
              # images are pushed to.
              - --project=k8s-staging-cluster-api
              # This is the same as above, but with -gcb appended.
              - --scratch-bucket=gs://k8s-staging-cluster-api-gcb
              - --env-passthrough=PULL_BASE_REF
              - .
```

[gcr instructions]: https://github.com/kubernetes/k8s.io/blob/master/k8s.gcr.io/README.md
[gcb documentation]: https://cloud.google.com/cloud-build/docs/configuring-builds/create-basic-configuration
[gcb-docker-gcloud]: https://github.com/kubernetes/test-infra/blob/master/images/gcb-docker-gcloud/Dockerfile
[substitution docs]: https://cloud.google.com/cloud-build/docs/configuring-builds/substitute-variable-values#using_user-defined_substitutions
