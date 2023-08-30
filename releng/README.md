# Release Engineering tooling <!-- omit in toc -->

This directory contains tooling to generate Prow jobs.
While some of this may be generically useful for other cases, the primary
function of these tools is to generate release branch jobs after
kubernetes/kubernetes releases which create new branches.

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

- [`generate_tests.py`](./generate_tests.py): Populates generated jobs based on
  the configurations specified in [`test_config.yaml`](./test_config.yaml)
- [`prepare_release_branch.py`](./releng/prepare_release_branch.py): Generates
  release branch jobs using `config-forker` and `config-rotator`
- [`config-forker`](./config-forker/README.md): Forks presubmit, periodic, and
  postsubmit job configs with the `fork-per-release` annotation
- [`config-rotator`](./config-rotator/README.md): Rotates forked presubmit,
  periodic, and postsubmit job configs created by `config-forker`

## Release branch jobs

**WARNING:** Release branch jobs generation for 1.28+ requires special steps
that are yet to be documented. See [#29387](https://github.com/kubernetes/test-infra/pull/29387)
and "TODO(1.29)" comments for more details.

This task should be done after the release is complete and previous PRs are
merged. The following steps should be run from the root of this repository.

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
