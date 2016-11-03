## How it works

1. We run a GKE cluster in the `kubernetes-jenkins-pull` project.
1. On this cluster, we run a deployment/service that listens for GitHub
   webhooks (`cmd/hook`). When an event comes in, we pass it off to registered
   plugins which can take actions based on it.
1. The `trigger` plugin listens for test commands and starts a Kubernetes job
   (`cmd/line`) which updates the GitHub status line.
1. We garbage collect old jobs and pods using `cmd/sinker`.
1. We provide a simple dashboard using `cmd/deck` that isn't quite useful yet.

## How to update the cluster

Any modifications to Go code will require redeploying the affected binaries.
Fortunately, this should result in no downtime for the system. Bump the
relevant version number in the makefile as well as in the `cluster` manifest
and run `make update-cluster`.

**Please ensure that your git tree is up to date before updating anything.**

## How to add new plugins

Add a new package under `plugins` with a method satisfying one of the handler
types in `plugins`. In that package's `init` function, call
`plugins.Register*Handler(name, handler)`. Then, in `cmd/hook/main.go`, add an
empty import so that your plugin is included. If you forget this step then a
unit test will fail when you try to add it to `plugins.yaml`. Don't add a brand
new plugin to the main `kubernetes/kubernetes` repo right away, start with
somewhere smaller and make sure it is well-behaved.

The LGTM plugin is a good place to start.

## How to enable a plugin on a repo

Add an entry to `plugins.yaml`. If you misspell the name then a unit test will
fail. Once it is merged, run `make update-plugins`. This does not require
redeploying the binaries, and will take effect within a minute.

## How to add new jobs

To add a new job you'll need to add an entry into `jobs.yaml`. Then run `make
update-jobs`. This does not require redeploying any binaries, and will take
effect within a minute.

The Jenkins job itself should have no trigger. It will be called with string
parameters `PULL_NUMBER` and `PULL_BASE_REF` which it can use to checkout the
appropriate revision. It needs to accept the `buildId` parameter which the
`line` job uses to track its progress.

## Bots home

[@k8s-ci-robot](https://github.com/k8s-ci-robot) and its silent counterpart
[@k8s-bot](https://github.com/k8s-bot) both live here as triggers to GitHub
messages defined in [jobs.yaml](jobs.yaml).
