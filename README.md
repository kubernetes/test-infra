## Overview

Mungegithub provides a number of tools intended to automate github processes. While mainly for the kubernetes community, some thought is put into making it generic. Mungegithub is built as a single binary, but is run in 3 different ways for 3 different purposes.

1. submit-queue: This looks at open PRs and attempts to help automate the process getting PRs from open to merged.
2. cherrypick: This looks at open and closed PRs with the `cherry-pick-candidate` label and attempts to help the branch managers deal with the cherry-pick process.
3. shame-mailer: This looks at open issues and e-mails assignees who have not closed their issues rapidly.

One can see the specifics of how the `submit-queue` and `cherrypick` options are executed by looking at the deployment definition in their respective subdirectories.

One may also look in the `example-one-off` directory for a small skeleton program which prints the number of all open PRs. It is an excellent place to start if you need to write a 'one-off' automation across a large number of PRs.

## Building and running

Executing `make help` inside the `mungegithub` directory should inform you about the functions provided by the Makefile. A common pattern when developing is to run something like:
```sh
make mungegithub && ./mungegithub --dry-run --token-file=/path/to/token --once --www=submit-queue/www --kube-dir=$GOPATH/src/k8s.io/kubernetes --pr-mungers=submit-queue --min-pr-number=25000 --max-pr-number=25500 --organization=kubernetes --project=kubernetes --repo-dir=/tmp
```

A Github oauth token is required, even in readonly/test mode. For production, we use a token with write access to the repo in question. https://help.github.com/articles/creating-an-access-token-for-command-line-use/ discusses the procedure to get a personal oauth token. These tokens will need to be loaded into kubernetes `secret`s for use by the submit and/or cherry-pick queue. It is extremely easy to use up the 5,000 API calls per hour so the production token should not be re-used for tests.

After successfully running the local binary one may build, test, and deploy in readonly mode to a real kube cluster. To do so, one must make sure that one's kubeconfig file is set up for the test/read-only cluster by running any necessary `kubectl config` commands by hand. One may also need a container repository with read & write access. It is just as easy to create a new dockerhub account, and a public repository named `submit-queue` within that. We will refer to the repository as `docker.io/$USERNAME` where `$USERNAME` is a placeholder. The steps required to deploy on a real cluster for the submit-queue application are as follows. The instructions to run the cherrypick application are along the same lines. Below, we explain the steps to run on the kubernetes main repository. Running on other repositories is similar, except that the corresponding YAML files are in a directory for that repository.

- Store your personal access token in a plain text file named (token) in the mungegithub directory.
- Run `APP=submit-queue; TARGET=<reponame>; make secret` to generate a local.secret.yaml.
- Run `kubectl --kubeconfig=... create -f mungegithub/submit-queue/deployment/<reponame>/local.secret.yaml` to load the secret.
- Run `kubectl --kubeconfig=... create -f mungegithub/submit-queue/deployment/<reponame>/pv.yaml` to create a persistent volume. (If you are running a local cluster, and not on GCP, use `mungegithub/submit-queue/pv-local.yaml` to create a persistent volume on your host. The file may need to be modified to match the expected name of the persistent volume by the deployment). 
- Run `kubectl --kubeconfig=... create -f mungegithub/submit-queue/deployment/<reponame>/pvc.yaml` to create a persistent volume claim.
- Check that the persistent volume claim is bound by checking `kubectl --kubeconfig=... get pvc`.

After these steps, we may need to push a configmap, in case any of the commandline arguments were changed. Pushing a new configmap for the kubernetes repository looks like the following:
```sh
TARGET=kubernetes APP=submit-queue KUBECONFIG=/path/to/kubeconfig make push_config
```

It can finally deployed as:
```sh
TARGET=kubernetes REPO=docker.io/$USERNAME APP=submit-queue KUBECONFIG=/path/to/kubeconfig make deploy
```

After this has successfully deployed to the test cluster in read-only mode, running in production involves running any required `kubectl config` commands to point to the production cluster, pushing a configmap if necessary, and then running:
```sh
TARGET=kubernetes REPO=docker.io/$USERNAME APP=submit-queue KUBECONFIG=/path/to/kubeconfig READONLY=false make deploy
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
