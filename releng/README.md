# Release Engineering tooling <!-- omit in toc -->

This directory contains tooling to fork and rotate Prow jobs for Kubernetes
release branches. Jobs annotated with `fork-per-release` are automatically
forked for new branches and rotated through stability tiers.

> NOTE: The documentation here supersedes overlapping guidance from the
[Release Manager handbooks][branch-manager-handbook] (which will be removed at
a future date).

- [Tools](#tools)
- [Release branch jobs](#release-branch-jobs)
  - [Generate jobs](#generate-jobs)
  - [Update release dashboards](#update-release-dashboards)
  - [Check/resolve configuration errors](#checkresolve-configuration-errors)
  - [Create a pull request](#create-a-pull-request)
  - [Validate](#validate)
  - [Announce](#announce)

## Tools

- [`prepare-release-branch`](./prepare-release-branch/): Orchestrates
  release branch preparation using `config-rotator` and `config-forker`.
  The tool is idempotent and will exit early if the next release branch
  does not exist yet.
- [`config-forker`](./config-forker/README.md): Forks presubmit, periodic, and
  postsubmit job configs with the `fork-per-release` annotation. Also
  importable as a Go package (`config-forker/pkg`).
- [`config-rotator`](./config-rotator/README.md): Rotates forked presubmit,
  periodic, and postsubmit job configs created by `config-forker`. Also
  importable as a Go package (`config-rotator/pkg`).

## Release branch jobs

This task should be done after the release branch has been created and
previous PRs are merged. The following steps should be run from the root of
this repository.

### Generate jobs

```console
make -C releng prepare-release-branch
```

### Update release dashboards

Update release dashboards in the [Testgrid config](https://git.k8s.io/test-infra/config/testgrids/kubernetes/sig-release/config.yaml) ([example commit](https://github.com/kubernetes/test-infra/pull/15023/commits/cad8a3ce8ef3537568b12619634dff702b16cda7)).

- Remove the oldest release `sig-release-<version>-{blocking,informing}` dashboards
- Add dashboards for the current release e.g., `sig-release-1.23-{blocking,informing}`

### Check/resolve configuration errors

```console
make verify
```

### Create a pull request

Issue a PR with the new release branch job configurations ([example PR](https://github.com/kubernetes/test-infra/pull/15023)).

### Validate

Once the PR has merged, verify that the new dashboards have been created and are populated with jobs.

Examples:

- [sig-release-1.27-blocking](https://testgrid.k8s.io/sig-release-1.27-blocking)
- [sig-release-1.27-informing](https://testgrid.k8s.io/sig-release-1.27-informing)

### Announce

[Announce in #sig-release and #release-management](https://kubernetes.slack.com/archives/C2C40FMNF/p1565746110248300?thread_ts=1565701466.241200&cid=C2C40FMNF) that this work has been completed.

[branch-manager-handbook]: https://git.k8s.io/sig-release/release-engineering/role-handbooks/branch-manager.md
