## How it works

1. We run a small GKE cluster in the same project as PR Jenkins.
1. On this cluster, we run a deployment/service that listens for GitHub
   webhooks (`cmd/hook`). When an event of interest comes in, such as a new PR
   or an "ok to test" comment, we check that it's safe to test, and then start
   up the corresponding jobs.
1. The jobs themselves (`cmd/line`) start and watch the Jenkins job, setting
   the GitHub status line along the way.
1. We garbage collect old jobs and pods using `cmd/sinker`.
1. We provide a simple dashboard using `cmd/deck` that isn't quite useful yet.

## How to add new jobs

To add a new job you'll need to add an entry into `jobs.yaml`. Then run `make
update-jobs`.

The Jenkins job itself should have no trigger. It will be called with string
parameters `ghprbPullId` and `ghprbTargetBranch` which it can use to checkout
the appropriate revision. It needs to accept the `buildId` parameter which the
`line` job uses to track its progress.

## Setup

To start it up look over the `create-cluster` rule in the makefile. You will
need to add the loadbalancer ingress address to any GitHub repos you want it
to track. It only needs `pull_request` and `issue_comment` events. The OAuth
token needs write access to the repository or you'll see lots of 404s in the
logs. Note that even if there are no jobs defined for the repo, it will still
make "is this PR ok to test" comments.

## Bots home

[@k8s-ci-robot](https://github.com/k8s-ci-robot) and its silent counterpart
[@k8s-bot](https://github.com/k8s-bot) both live here as triggers to GitHub
messages defined in [jobs.yaml](jobs.yaml).
