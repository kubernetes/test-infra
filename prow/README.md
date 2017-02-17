# Prow

Prow is the system that handles GitHub events and commands for Kubernetes. It
currently comprises several related pieces that live in a GKE cluster.

* `cmd/hook` is the most important piece. It is a server that listens for
  GitHub webhooks and dispatches them to the appropriate handlers.
* `cmd/line` is the piece that starts Jenkins jobs or k8s pods.
* `cmd/sinker` cleans up old jobs and pods.
* `cmd/splice` regularly schedules batch jobs.
* `cmd/deck` presents [a nice view](https://prow.k8s.io/) of recent jobs.
* `cmd/phony` sends fake webhooks.
* `cmd/marque` is a production-ready letsencrypt certificate manager.
* `cmd/tot` vends incrementing build numbers.
* `cmd/crier` writes GitHub statuses and comments.

## How to test prow

Build with `bazel build //prow/...`, test with `bazel test //prow/...`.

You can run `cmd/hook` in a local mode for testing, and hit it with arbitrary
fake webhooks. To do this, run `./bazel-bin/prow/cmd/hook/hook --local
--config prow/config.yaml --plugin-config prow/plugins.yaml` in one shell. This
will listen on `localhost:8888` for webhooks. Send one with
`./bazel-bin/prow/cmd/phony/phony --event issue_comment --payload
prow/cmd/phony/examples/test_comment.json`.

## How to update the cluster

Any modifications to Go code will require redeploying the affected binaries.
Fortunately, this should result in no downtime for the system. Bump the
relevant version number in the makefile as well as in the `cluster` manifest
and run `make update-cluster`. You can also consider updating individual
images and deployments, if you'd like.

**Please ensure that your git tree is up to date before updating anything.**

## How to add new plugins

Add a new package under `plugins` with a method satisfying one of the handler
types in `plugins`. In that package's `init` function, call
`plugins.Register*Handler(name, handler)`. Then, in `cmd/hook/main.go`, add an
empty import so that your plugin is included. If you forget this step then a
unit test will fail when you try to add it to `plugins.yaml`. Don't add a brand
new plugin to the main `kubernetes/kubernetes` repo right away, start with
somewhere smaller and make sure it is well-behaved. If you add a command,
document it in [commands.md](./commands.md).

The LGTM plugin is a good place to start if you're looking for an example
plugin to mimic.

## How to enable a plugin on a repo

Add an entry to `plugins.yaml`. If you misspell the name then a unit test will
fail. Once it is merged, run `make update-plugins`. This does not require
redeploying the binaries, and will take effect within a minute.

## How to add new jobs

To add a new job you'll need to add an entry into `config.yaml`. Then run `make
update-config`. This does not require redeploying any binaries, and will take
effect within a minute.

The Jenkins job itself should have no trigger. It will be called with string
parameters `PULL_NUMBER` and `PULL_BASE_REF` which it can use to checkout the
appropriate revision. It needs to accept the `buildId` parameter which the
`line` job uses to track its progress.

## Bots home

[@k8s-ci-robot](https://github.com/k8s-ci-robot) and its silent counterpart
[@k8s-bot](https://github.com/k8s-bot) both live here as triggers to GitHub
messages defined in [config.yaml](config.yaml).
