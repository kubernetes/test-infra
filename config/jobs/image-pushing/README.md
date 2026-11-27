# Image pushing jobs

This directory contains jobs that run in the trusted cluster and kick off GCB
jobs that then push images to staging GCR/AR repos. These jobs are the recommended
way to regularly publish images to staging.

## Getting started

You'll need a staging GCR. If you don't have one,
[instructions are over here][registry instructions]. Once you have one, there are two
components two getting set up:

* A `cloudbuild.yaml` file in your repo, customised to build your images in
  whatever way works for you
* A cookie-cutter prow job config in this directory. These are almost the same
  for all builds, and variance should be avoided wherever possible.

## cloudbuild.yaml

The contents of `cloudbuild.yaml` depends on how your repo produces images.
The [official documentation][gcb documentation] discusses these in the general
case. If your image can be built using `go build` and pushed using
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


### Build example

If your build process is driven by a Makefile or similar, you can use GCB to
invoke that. We provide the [`gcr.io/k8s-staging-test-infra/gcb-docker-gcloud` image][gcb-docker-gcloud],
which contains components that are likely to be useful for your builds. This image supports multiarch docker builds without additional commands.
A sample `cloudbuild.yaml` using `make` to build and push might look like this:

```yaml
# See https://cloud.google.com/cloud-build/docs/build-config

# this must be specified in seconds. If omitted, defaults to 600s (10 mins)
timeout: 1200s
# this prevents errors if you don't use both _GIT_TAG and _PULL_BASE_REF,
# or any new substitutions added in the future.
options:
  substitution_option: ALLOW_LOOSE
steps:
  - name: gcr.io/k8s-staging-test-infra/gcb-docker-gcloud:latest
    env:
    - TAG=$_GIT_TAG
    - BASE_REF=$_PULL_BASE_REF
    args:
    - make
    - build
substitutions:
  # _GIT_TAG will be filled with a git-based tag for the image, of the form vYYYYMMDD-hash, and
  # can be used as a substitution
  _GIT_TAG: '12345'
  # _PULL_BASE_REF will contain the ref that was pushed to to trigger this build -
  # a branch like 'master' or 'release-0.2', or a tag like 'v0.2'.
  _PULL_BASE_REF: 'main'
serviceAccount: projects/k8s-staging-images/serviceAccounts/<project-name>-sa@k8s-staging-images.iam.gserviceaccount.com
```

Don't override the entrypoint as it does some important bootstrapping such as docker buildx multiarch support.

## Prow config template

These jobs run in the trusted cluster. As such, we will not accept variants that
run arbitrary code inside the prow job, or jobs that substantially deviate from
this template. One day, we may automate this away.

If you aren't sure, feel free to ask a [reviewer](./OWNERS) to do this part
for you. 

Prow config should be in a file named `k8s-staging-<project-name>.yaml`, and should
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
        testgrid-dashboards: sig-something-image-pushes, sig-k8s-infra-gcb
      decorate: true
      # this causes the job to only run on the main branch. Remove it if your
      # job makes sense on every branch (unless it's setting a `latest` tag it
      # probably does).
      # if you remove it you must instead use the following:
      #skip_branches:
      #  # do not run on dependabot branches, these exist prior to merge
      #  # only merged code should trigger these jobs
      #  - '^dependabot'
      branches:
        - ^main$
        # Build release-* branches
        - ^release-
        # Build semver tags, too
        - ^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$
      spec:
        serviceAccountName: <project-name>
        containers:
          - image: gcr.io/k8s-staging-test-infra/image-builder:v20261013-5cade36dc1
            command:
              - /run.sh
            args:
              - --project=k8s-staging-images
              - --scratch-bucket=gs://k8s-staging-images-gcb
              - --env-passthrough=PULL_BASE_REF
              - .
            resources:
              requests:
                cpu: 1
                memory: 1Gi
              limits:
                cpu: 1
                memory: 1Gi
```

If you need the `.git` directory to be uploaded to your build environment:
- pass the `--with-git-dir` command-line option to `run.sh`;
- add an empty `.gcloudignore` file to your repository. This will override the
  [default values][gcloudignore].

## Migrating from legacy `gcr.io/k8s-staging-*` projects

When migrating an existing project from a dedicated `k8s-staging-<project-name>` GCP project (`gcr.io/k8s-staging-<project-name>`) and `serviceAccountName: gcb-builder` to the shared `k8s-staging-images` Artifact Registry ([kubernetes/k8s.io#9486]):

1. **Provision the registry and dedicated ServiceAccount in `kubernetes/k8s.io`:** Add `<project-name>` to `locals.registries` and `locals.skip_gcb_builder_shim` in `infra/gcp/terraform/k8s-staging-images/registries.tf`, run `hack/verify-staging-builds-sa.sh`, and update `registry.k8s.io/manifests/k8s-staging-<project-name>/promoter-manifest.yaml` once applied.
2. **Prepare the repository-side PR(s) first:** Update `cloudbuild.yaml` to set `serviceAccount: projects/k8s-staging-images/serviceAccounts/<project-name>-sa@k8s-staging-images.iam.gserviceaccount.com` and update the target registry in your build scripts/`Makefile` to `us-central1-docker.pkg.dev/k8s-staging-images/<project-name>`. If the Prow job also builds release branches (`^release-`), prepare the corresponding cherry-picks for active release branches.
3. **Coordinate the `test-infra` cutover with project maintainers:** Because the Prow job (`--project=k8s-staging-images`) and `cloudbuild.yaml` (`serviceAccount: ...`) live in separate repositories, merging one without the other will cause postsubmit builds to fail until both land. Always `/cc` the subproject `OWNERS` / tech leads on the `test-infra` PR and place a `/hold` on it until the repository-side PR is reviewed and ready to merge alongside it.

[registry instructions]: https://github.com/kubernetes/k8s.io/blob/main/registry.k8s.io/README.md
[gcb documentation]: https://cloud.google.com/cloud-build/docs/configuring-builds/create-basic-configuration
[gcb-docker-gcloud]: https://github.com/kubernetes/test-infra/blob/master/images/gcb-docker-gcloud/Dockerfile
[gcloudignore]: https://cloud.google.com/sdk/gcloud/reference/topic/gcloudignore
[substitution docs]: https://cloud.google.com/cloud-build/docs/configuring-builds/substitute-variable-values#using_user-defined_substitutions
[kubernetes/k8s.io#9486]: https://github.com/kubernetes/k8s.io/issues/9486
