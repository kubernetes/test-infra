## Overview

Mungegithub provides a number of tools intended to automate github processes. While mainly for the kubernetes community, some thought is put into making it generic. Mungegithub is built as a single binary, but is run in 3 different ways for 3 different purposes.

1. submit-queue: This looks at open PRs and attempts to help automate the process getting PRs from open to merged.
2. cherrypick: This looks at open and closed PRs with the `cherry-pick-candidate` label and attempts to help the branch managers deal with the cherry-pick process.
3. shame-mailer: This looks at open issues and e-mails assignees who have not closed their issues rapidly.

One can see the specifics of how the `submit-queue` and `cherrypick` options are executed by looking at the deployment definition in their respective subdirectories.

One may also look in the `example-one-off` directory for a small skeleton program which prints the number of all open PRs. It is an excellent place to start if you need to write a 'one-off' automation across a large number of PRs.

## Building and running

Executing `make help` inside the `mungegithub` directory should inform you about the functions provided by the Makefile. A common pattern for me when developing is to run something like:
```sh
make mungegithub && ./mungegithub --dry-run --token-file=/path/to/token --once --www=submit-queue/www --kube-dir=$GOPATH/src/k8s.io/kubernetes --pr-mungers=submit-queue --min-pr-number=25000 --max-pr-number=25500
```

I typically have both a test and a prod cluster. However there is no reason a test/prod instance couldn't be done on the same cluster in 2 namespaces. It's just how your kubeconfig is set up. You will need a github oauth, even in readonly/test mode. Obviously for production you will want a token with write access to the repo in question. https://help.github.com/articles/creating-an-access-token-for-command-line-use/ should discuss how to get an oauth token. These tokens will need to be loaded into kubernetes `secret`s for use by the submit and/or cherry-pick queue. It is extremely easy to use up the 5,000 API calls per hour so the production token should not be re-used for tests.

After successfully running the local binary I will typically build, test, and deploy in readonly mode to a real kube cluster. To do so you must make sure your kubeconfig file is set up for the test/read-only cluster by running any necessary `kubectl config` commands by hand. After which it is as simple as something like:
```sh
REPO=docker.io/eparis APP=submit-queue KUBECONFIG=/path/to/kubeconfig make deploy
```

After this has successfully deployed to the test cluster in read-only mode running in production required running any required `kubectl config` commands and then running
```sh
REPO=docker.io/eparis APP=submit-queue KUBECONFIG=/path/to/kubeconfig READONLY=false make deploy
```

## About the mungers

A small amount of information about some of the individual mungers inside each of the 3 varieties are listed below:

### submit-queue
* block-paths - add `do-not-merge` label to PRs which change files which should not be changed (mainly old docs moved to kubernetes.github.io)
* blunderbuss - assigned PRs to individuals based on the contents of OWNERS files in the main repo
* cherrypick-auto-approve - adds `cherrypick-approved` to PRs in a release branch if the 'parent' pr in master was approved
* cherrypick-label-unapproved - adds `do-not-merge` label to PRs against a release-\* branch which do not have `cherrypick-approved`
* comment-deleter - deletes comments created by the k8s-merge-robot which are no longer relevant. Such as comments about a rebase being required if it has been rebased.
* comment-deleter-jenkins - deleted comments create by the k8s-bot jenkins bot which are no longer relevant. Such as old test results.
* lgtm-after-commit - removes `lgtm` label if a PR is changed after the label was added
* needs-rebase - adds and removes a `needs-rebase` label if a PR needs to be rebased before it can be applied.
* path-label - adds labels, such as `kind/new-api` based on if ANY file which matches changed
* rebuild-request - Looks for requests to retest a PR but which did not reference a flake.
* release-note-label - Manages the addition/removal of `release-note-label-required` and all of the rest of the `release-note-*` labels.
* size - Adds the xs/s/m/l/xl labels and comments to PRs
* stale-green-ci - Reruns the CI tests every X hours (96?) for PRs which passed. So PRs which sit around for a long time will notice failures sooner.
* stale-pending-ci - Reruns the CI tests if they have been 'in progress'/'pending' for 24 hours.
* submit-queue - This is the brains that actually tracks and merges PRs. It also provides the web site interface.

### cherrypick
* cherrypick-clear-after-merge - This watches for PRs against release branches which merged and removes the `cherrypick-candidate` label from the PR on master.
* cherrypick-must-have-milestone - This complains on any PR against a release branch which does not have a vX.Y milestone.
* cherrypick-queue - This is the web display of all PRs with the `cherrypick-candidate` label which a branch owner is likely to want to pay attention to.
