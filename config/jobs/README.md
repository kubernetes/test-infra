# Kubernetes Prow Job Configs

All prow job configs for [prow.k8s.io] live here.

They are tested and validated by tests in [`config/tests`](/config/tests)

## Job Cookbook

This document attempts to be a step-by-step, copy-pastable guide to the use
of prow jobs for the Kubernetes project. It may fall out of date. For more
info, it was sourced from the following:

- [ProwJob docs](/prow/jobs.md)
- [Life of a Prow Job](/prow/life_of_a_prow_job.md)
- [Pod Utilities](/prow/pod-utilities.md)
- [How to Test a Prow Job](/prow/build_test_update.md#How-to-test-a-ProwJob)

### Job Types

There are three types of prow jobs:

- **Presubmits** run against code in PRs
- **Postsubmits** run after merging code
- **Periodics** run on a periodic basis

Please see [ProwJob docs](/prow/jobs.md) for more info

### Job Images

Where possible, we prefer that jobs use images that are pinned to a specific
version, containing only what is needed.

Some good examples include:

- [pull-release-unit] uses `golang:1.12` to run `go test ./...`
- [pull-release-notes-lint] uses `node:11` to run `npm ci && npm lint`
- [pull-org-test-all] uses `launcher.gcr.io/google/bazel:0.26.0` to run `bazel test //...`

Many jobs use `gcr.io/k8s-testimages/foo` images that are built from source in
[`images/`]. Some of these have evolved organically, with way more dependencies
than needed, and will be periodically bumped by PRs. These are sources of
technical debt that are often not very well maintained. Use at your own risk,
eg:

- [pull-community-verify] uses `gcr.io/k8s-testimages/gcloud-in-go:v20190125`
  to run `make verify`, which ends up invoking some `bash` scripts, which use
  commands like `git` and `go`. The `gcloud` dependency is not needed at all.
- [periodic-kubernetes-e2e-packages-pushed] uses `gcr.io/k8s-testimages/kubekins:latest-master`
  to run `./tests/e2e/packages/verify_packages_published.sh` which ends up
  running `apt-get` and `yum` commands. Perhaps a `debian` image would be
  better.

## Job Presets

Prow supports [Presets](/prow/jobs.md#presets) to define and patch in common
env vars and volumes used for credentials or common job config. Some are
defined centrally in [`config/prow/config.yaml`], while others can be defined in
files here. eg:

- [`preset-service-account: "true"`] ensures the prowjob has a GCP service
  account in a well known location, with well known env vars pointing to it.
- [`preset-pull-kubernetes-e2e: "true"`] sets environment variables to make
  kubernetes e2e tests less susceptible to flakes
- [`preset-aws-credentials: "true"`] ensures the prowjob has AWS credentials
  for kops tests in a well known location, with an env var pointint to it
- [the default preset with no labels] is used to set the `GOPROXY` env var
  for all jobs by default

## Job Examples

A presubmit job named "pull-community-verify" that will run against all PRs to
kubernetes/community's master branch. It will run `make verify` in a checkout
of kubernetes/community at the PR's HEAD. It will report back to the PR via a
status context named `pull-kubernetes-community`. Its logs and results are going
to end up in GCS under `kubernetes-jenkins/pr-logs/pull/community`. Historical
results will display in testgrid on the `sig-contribex-community` dashboard
under the `pull-verify` tab

```yaml
presubmits:
  kubernetes/community:
  - name: pull-community-verify  # convention: (job type)-(repo name)-(suite name)
    annotations:
      testgrid-dashboards: sig-contribex-community
      testgrid-tab-name: pull-verify
    branches:
    - master
    decorate: true
    always_run: true
    spec:
      containers:
      - image: golang:1.12.5
        command:
        - /bin/bash
        args:
        - -c
        # Add GOPATH/bin back to PATH to workaround #9469
        - "export PATH=$GOPATH/bin:$PATH && make verify"
```

A periodic job named "periodic-cluster-api-provider-aws-test-creds" that will
run every 4 hours against kubernetes-sigs/cluster-api-provider-aws's master
branch. It will run `./scripts/ci-aws-cred-test.sh` in a checkout of the repo
located at `sigs.k8s.io/cluster-api-provider-aws`. The presets it's using will
ensure it has aws credentials and aws ssh keys in well known locations. Its
logs and results are going to end up in GCS under 
`kubernetes-jenkins/logs/periodic-cluster-api-provider-aws-test-creds`.
Historical results will display in testgrid on the `sig-cluster-lifecycle-cluster-api-provider-aws`
dashboard under the `test-creds` tab

It's using the `kubekins-e2e` image which [isn't recommended](#job-images),
but works for now.

```yaml
periodics:
- name: periodic-cluster-api-provider-aws-test-creds
  annotations:
    testgrid-dashboards: sig-cluster-lifecycle-cluster-api-provider-aws
    testgrid-tab-name: test-creds
  decorate: true
  interval: 4h
  labels:
    preset-service-account: "true"
    preset-aws-ssh: "true"
    preset-aws-credential: "true"
  extra_refs:
  - org: kubernetes-sigs
    repo: cluster-api-provider-aws
    base_ref: master
    path_alias: "sigs.k8s.io/cluster-api-provider-aws"
  spec:
    containers:
    - image: gcr.io/k8s-testimages/kubekins-e2e:v20200428-06f6e3b-1.15
      command:
      - "./scripts/ci-aws-cred-test.sh"
```

## Adding or Updating Jobs

1. Find or create the prowjob config file in this directory
    - In general jobs for `github.com/org/repo` use `org/repo/filename.yaml`
    - For kubernetes/kubernetes we prefer `kubernetes/sig-foo/filename.yaml`
    - Ensure `filename.yaml` is unique across the config subdir; prow uses this as a key in its configmap
1. Ensure an [`OWNERS`](https://go.k8s.io/owners) file exists in the directory for job, and has appropriate approvers/reviewers
1. Write or edit the job config (please see [how-to-add-new-jobs](/prow/jobs.md#how-to-configure-new-jobs))
1. Ensure the job is configured to to display its results in [testgrid.k8s.io]
    - The simple way: add [testgrid annotations]
    - Please see the testgrid [documentation](/testgrid/config.md) for more details on configuation options
1. Open a PR with the changes; when it merges [@k8s-ci-robot] will deploy the changes automatically

## Deleting Jobs

1. Find the prowjob config file in this directory
1. Remove the entry for your job; if that was the last job in the file, remove the file
1. If the job had no [testgrid annotations], ensure its [`testgrid/config.yaml`] entries are gone
1. Open a PR with the changes; when it merges [@k8s-ci-robot] will deploy the changes automatically

## Testing Jobs Locally

Please try using [`phaino`](/prow/cmd/phaino/README.md), it will interactively
help you run a docker command that approximates the pod that would be scheduled
on behalf of an actual prow job.

## Testing Jobs Remotely

This requires a running instance of prow. In general we discourage the use of
[prow.k8s.io] as a testbed for job development, and recommend the use of your
own instance of prow for faster iteration. That said, an approach that people
have used in the past with mostly-there jobs is to iterate via PRs; just
recognize this is going to depend on review latency.

For more details, please refer to [How to Test a ProwJob](/prow/build_test_update.md#how-to-test-a-prowjob)

## Running a Production Job

Normally prow will automatically schedule your job, however if for some reason you
need to trigger it again and are a Prow administrator you have a few options:

- you can use the rerun feature in prow.k8s.io to run the job again *with the same config*
- you can use [`config/mkpj.sh`](/config/mkpj.sh) to create a prowjob CR from your local config
- you can use `bazel run //prow/cmd/mkpj -- --job=foo ...` to create a prowjob CR from your local config

For the latter two options you'll need to submit the resulting CR via `kubectl` configured against
the prow services cluster.

## Generated Jobs

There are some sets of jobs that are generated and should not be edited by hand.
These specific instructions should probably just live adjacent to the jobs rather
than in this central README, but here we are for now.

### image-validation jobs

These test different master/node image versions against multiple k8s branches. If you
want to change these, update [`releng/test_config.yaml`](/releng/test_config.yaml)
and then run

```sh
# from test-infra root
$ ./hack/update-generated-tests.sh
```

### release-branch jobs

When a release branch of kubernetes is first cut, the current set of master jobs
must be forked to use the new release branch. Use [`releng/config-forker`] to
accomplish this, eg:

```sh
# from test-infra root
$ bazel run //releng/config-forker -- \
  --job-config $(pwd)/config/jobs \
  --version 1.15 \
  --output $(pwd)/config/jobs/kubernetes/sig-release/release-branch-jobs/1.15.yaml
```

[prow.k8s.io]: https://prow.k8s.io
[@k8s-ci-robot]: https://github.com/k8s-ci-robot
[testgrid annotations]: /testgrid/config.md#prow-job-configuration
[testgrid.k8s.io]: https://testgrid.k8s.io

[`releng/config-forker`]: /releng/config-forker
[`images/`]: /images

[periodic-kubernetes-e2e-packages-pushed]: https://github.com/kubernetes/test-infra/blob/688d365adf7f71e33a4249c7b90d7e84c105dfc5/config/jobs/kubernetes/sig-cluster-lifecycle/packages.yaml#L3-L16
[pull-community-verify]: https://github.com/kubernetes/test-infra/blob/f4e6553b27d9ee8b35b2f2e588ea2e18c3fa818b/config/jobs/kubernetes/community/community-presubmit.yaml#L3-L19
[pull-release-unit]: https://github.com/kubernetes/test-infra/blob/294d73f1e1c87e6b93f60287196438325bc35677/config/jobs/kubernetes/release/release-config.yaml#L37
[pull-release-notes-lint]: https://github.com/kubernetes/test-infra/blob/294d73f1e1c87e6b93f60287196438325bc35677/config/jobs/kubernetes-sigs/release-notes/release-notes-presubmits.yaml#L69-L80
[pull-org-test-all]: https://github.com/kubernetes/test-infra/blob/294d73f1e1c87e6b93f60287196438325bc35677/config/jobs/kubernetes/org/kubernetes-org-jobs.yaml#L3-L13

[`preset-service-account: "true"`]: https://github.com/kubernetes/test-infra/blob/f4e6553b27d9ee8b35b2f2e588ea2e18c3fa818b/prow/config.yaml#L467-L483
[`preset-pull-kubernetes-e2e: "true"`]: https://github.com/kubernetes/test-infra/blob/f4e6553b27d9ee8b35b2f2e588ea2e18c3fa818b/config/jobs/kubernetes/sig-gcp/sig-gcp-gce-config.yaml#L2-L8
[`preset-aws-credentials: "true"`]: https://github.com/kubernetes/test-infra/blob/f4e6553b27d9ee8b35b2f2e588ea2e18c3fa818b/config/jobs/kubernetes/sig-aws/sig-aws-credentials.yaml#L3-L15
[the default preset with no labels]: https://github.com/kubernetes/test-infra/blob/551edb4702e262989fe5d162a2c642c3201bf68e/prow/config.yaml#L630
