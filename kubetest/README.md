# Kubetest

Kubetest is the interface for launching and running e2e tests.

See the contributor documentation for information about [e2e testing].

Kubetest sits between [bootstrap.py] and various parts of the e2e lifecycle.

The `bootstrap.py` library is a nominal/optional part of [prow].
This library is responsible for:
* checking out each repository correctly
* starting kubetest (or whatever the test binary is for other jobs)
* uploading the test result (including artifacts) to gcs

The e2e lifecycle may:
* `--build` kubernetes,
* `--stage` this build to gcs,
* `--extract` a staged build from gcs,
* turn `--up` a new cluster using various `--deployment` strategies,
* `--test` this cluster using [ginkgo] `--test_args`
* `--dump` logs to a local folder, and finally
* turn `--down` the cluster after completing testing,
* `--timeout` after a particular duration (allowing extra time to clean up).

If you're making a change to kubetest, watch the canary jobs after it has merged;
these run the latest images of `kubekins-e2e`, and thus don't have to wait
for a version bump in the prow config.  The canary jobs are used to give early
signal on whether new kubetest features are working as intended, before bumping
the image for general e2e tests.

## Installation

Please clone this repo and then run `GO111MODULE=on go install ./kubetest` from the clone.

Common alternatives:
```
go install k8s.io/test-infra/kubetest  # if you check out test-infra
bazel run //kubetest  # use bazel to build and run
```

### Releases

Right now `kubetest` is expected to run at head, regardless of the version of
kubernetes being targeted.

Most e2e images, such as [kubekins-e2e] and [kubekins-e2e-prow] compile the
latest version of kubetest whenever the image is updated (most updates to these
images are done in order to update kubetest).


## Build

If `PWD` is in the `kubernetes/kubernetes` directory `--build` will build
whatever changes you have made into a quick release.

Control the details of the `--build=bazel` by appending one of the build modes
(see help for current list).

### Stage a build

It is inefficient for every job to rebuild the same version. Instead our CI
system defines build jobs which `--stage` the build somewhere on GCS. Some
providers such as GKE require a staged build, whereas others like GCE allow you
to `scp` over the binaries directly to each node.


### Extract a build

Aside from the build jobs, most of our CI systems `--extract` a prebuilt
version. This saves a bunch of time compiling.

The most common options are either a specific version`--extract=v1.7.0-beta.1`,
a release `--extract=release/stable` or `--extract=ci/latest-1.8`.

Note that you can extract 1 or 2 versions. Using 2 versions is useful for skew
and upgrade testing.

See [extract_k8s.go] for further details.


## Cluster-lifecycle

There are various ways to deploy kubernetes. Choose a strategy with the
`--deployment` flag (for example `--deployment=kops`).
See `kubetest --help` for a full list of options.

### Up

The `--up` flag will tell `kubetest` to turn up a new cluster for you.

It will first attempt to tear down an old instance of the same cluster.

Currently requires a complicated set of flags and environment variables
such as `--gcp-project`, etc.

We are in the process of converting all environment variables into flags. See
the current set of flag options with `kubetest -h`.

#### Save/load credentials

The `--save` flag tells kubetest to upload your cluster credentials onto gcs
somewhere. Later calling `kubetest --save` without an `--up` flag tells kubetest
to load these credentials instead of turning up a new cluster.


#### Dynamic project selection

Most e2e jobs assume control of a GCP project (see leaks section below).

If `kubetest` is running inside a pod then it will attempt to talk to [boskos]
to dynamically reserve a project.

This makes it easier for developers to add and remove jobs. With boskos they no
longer need to worry about creating, provisioning, naming, etc a project for
this new job.

The `--boskos-wait-duration` flag defines how long Kubetest waits on an Boskos resource
becomes available before quitting the job, default value is 5 minutes.

See the boskos docs for more details.

### Dump logs

The `--dump` flag tells `kubetest` to try and collect logs if there is a
problem. Typically this means master and node logs.

Collecting these logs may take a long time. This typically involves sshing to
each node, searching for and downloading any relevant logs.

There is also a `--logexporter-gcs-path` option which tells `kubetest` to run a
container on each node which uploads logs directly to GCS. This dramatically
reduces time required to dump logs, especially for scalability tests.

### Down

The `--down` flag tells `kubetest` to clear up the cluster after finishing.

Kubetest will try its best to tear down the cluster in spite of problems such as
failing `--up`, `--test`, etc. The `--timeout` also includes some buffer to
allow time for `--down` to clean up.

#### Leaks

The `--check-leaked-resources` option tells kubetest to look for any extra `GCP`
resources after tearing down the cluster.

The expectation is that any resources created by kubernetes will be cleaned up
during cluster teardown.

This logic may be buggy so this options takes a snapshot of the resources at
various points in time (start, after cluster up, after testing, after cluster
down) and ensures that there are no resources present after down that weren't
already present at the start.


## Testing

Most testing uses ginkgo but there are other options available.

### Ginkgo

The `--test` flag tells `kubetest` to run the `test.e2e` binary built/extracted
from the `kubernetes/kubernetes` repo.

Typically jobs also include a `--test_args=--ginkgo.focus=FOO --ginkgo.skip=BAR`
flag to filter down to a particular set of interesting tests.

### Upgrade, skew, kubemark

You can also run `--kubemark` tests instead of the standard
tests.

Tests can use `--skew` and `--upgrade_args` if they provided multiple
`--extract` flags (or manually created a `kubernetes/kubernetes_skew` directory
as a sibling to `kubernetes/kubernetes`). This will cause tests to run from the
skew directory, potentially to upgrade/downgrade kubernetes to another version.

A simple example is:<br>
`kubetest --up --check-version-skew=false --extract=v1.8.0-beta.1
--extract=v1.7.5 --upgrade_args=--ginkgo.focus="Feature:SomeUpgrade"
--gcp-project=google.com:some-project`

The command runs all (and only) upgrade tests tagged with `Feature:SomeUpgrade`
label on GCE. The command downloads `v1.7.5` and `v1.8.0-beta.1` releases,
unzips downloaded files, and runs the tests to upgrade the cluster from `v1.7.5`
to `v1.8.0-beta.1`. You will be able to find 2 new directories named `kubernetes`
and `kubernetes_skew` at current directory. `kubernetes_skew` is the directory
corresponding to release indicated by first `--extract` flag, while `kubernetes`
corresponds to second flag.
The first `--extract` downloads and extracts the specified release into the `kubernetes`
directory. The second `--extract` renames the `kubernetes` directory as `kubernetes_skew`
and then downloads the second release into a new `kubernetes` directory.

Note that order of the 2 `--extract` flags matters: `--extract=v2 --extract=v1` means
upgrading from v1 to v2. The command does not run other e2e tests after completing
the upgrade tests. If you want to run the e2e tests, specify also `--test` and
`--test_args` flags.

Tips: CI upgrade tests listed at [sig-cluster-lifecycle config] show flags used in the real CI
test environment, which is a good source to learn more about how the flags are used.

### Staging

If you want to create a new release with your own changes, you have to upload built
manifests to gcs. The command is very similar:
`kubetest --build --stage=gs://some/path/to/v1.8.0-beta.1  --check-version-skew=false
--extract=gs://some/path/to/v1.8.0-beta.1  --extract=v1.7.5
--upgrade_args=--ginkgo.focus="Feature:SomeUpgrade"
--gcp-project=google.com:some-project`

If you already have release of a specific version, you do not need to fetch the
release again. For instance, if you have `v1.7.5` release and its directory is at the
right path, the command below does the same as above:
`kubetest --build --stage=gs://some/path/to/v1.8.0-beta.1  --check-version-skew=false
--extract=gs://some/path/to/v1.8.0-beta.1
--upgrade_args=--ginkgo.focus="Feature:SomeUpgrade"
--gcp-project=google.com:some-project`

[bootstrap.py]: /jenkins/bootstrap.py
[boskos]: /boskos
[e2e testing]: https://git.k8s.io/community/contributors/devel/sig-testing/e2e-tests.md
[extract_k8s.go]: /kubetest/extract_k8s.go
[ginkgo]: https://github.com/onsi/ginkgo
[kubekins-e2e]: /images/kubekins-e2e
[kubekins-e2e-prow]: /images/e2e-prow
[prow]: /prow
[sig-cluster-lifecycle config]: /config/jobs/kubernetes/sig-cluster-lifecycle
