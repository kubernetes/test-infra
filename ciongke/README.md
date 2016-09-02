## How it works

1. We run a small GKE cluster in the same project as PR Jenkins.
1. On this cluster, we run a deployment/service that listens for GitHub
   webhooks (`cmd/hook`). When an event of interest comes in, such as a new PR
   or an "ok to test" comment, we check that it's safe to test, and then start
   up the corresponding jobs.
1. The jobs themselves (`cmd/test-pr`) start and watch the Jenkins job, setting
   the GitHub status along the way.

## How to add new jobs

To add a new job you'll need to add an entry into `cmd/hook/main.go`. The
`Name` field is the name of the Jenkins job, the `Trigger` phrase is a regular
expression that commenters can use to run your test. The `Context` phrase is
how to denote your test in the GitHub status line. If `AlwaysRun` is `true`
then your test will run for every PR, otherwise it will only be triggered by
comments.

The Jenkins job itself should have no trigger. It will be called with string
parameters `ghprbPullId` and `ghprbTargetBranch` which it can use to checkout
the appropriate revision. It needs to accept the `buildId` parameter which the
`test-pr` job uses to track its progress.

You'll need to bump the `hook` version and update the cluster once your change
goes in. Eventually we'd like to store job configurations in a ConfigMap or
something similar.

## How to update the cluster

If you make a change to `hook` or `test-pr`, bump the version in the makefile
as well as in `hook_deployment.yaml`. Do not push yet, just make sure the code
compiles and passes tests. Once your PR is reviewed, run `make update-cluster`.
This requires that your `kubectl` points to the `ciongke` cluster in
`kubernetes-jenkins-pull`.

There shouldn't be any downtime for updates that don't reconfigure Jenkins
jobs.

## Setup

To start it up look over the `create-cluster` rule in the makefile. You will
need to add the loadbalancer ingress address to any GitHub repos you want it
to track. It only needs `pull_request` and `issue_comment` events. The OAuth
token needs write access to the repository or you'll see lots of 404s in the
logs. Note that even if there are no jobs defined for the repo, it will still
make "is this PR ok to test" comments.
