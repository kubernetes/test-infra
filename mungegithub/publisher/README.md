## Overview

The publish robot publishes the code in `k8s.io/kubernetes/staging` to their own repositories. It guarantees that the master branches of the published repositories are compatible, i.e., if a user `go get` a published repository in a clean GOPATH, the repo is guaranteed to work.

The robot is built with the mungers framework. Every 24 hours, it pulls the latest k8s.io/kubernetes changes and runs `git filter-branch` to distill the commits that affect a staging repo. Then it cherrypicks new commits to the target repo. It records the SHA1 of the last cherrypicked commits in `kubernetes-sha` file in the target repo.

The robot is also responsible to update the `Godeps/Godeps.json` and the `vendor/` directory for the target repos. 

## Playbook

### Publishing a new repo

* Create a (repoRules) in [mungegithub/mungers/publisher.go](https://github.com/kubernetes/test-infra/blob/master/mungegithub/mungers/publisher.go#L94-L254)

* Add a `publish_<repository_name>.sh` in [mungegithub/mungers/publish_scripts](https://github.com/kubernetes/test-infra/tree/master/mungegithub/mungers/publish_scripts)

* [Test and deploy the changes](#testing-and-deploying-the-robot)

### Publishing a new branch

* Update the (repoRules) in [mungegithub/mungers/publisher.go](https://github.com/kubernetes/test-infra/blob/master/mungegithub/mungers/publisher.go#L94-L254)

* [Test and deploy the changes](#testing-and-deploying-the-robot)

### Testing and deploying the robot

Currently we don't have tests for the robot. It relies on manual tests:

* Fork the repos you are going the publish. Run [fetch-all-latest-and-push.sh](util/fetch-all-latest-and-push.sh) to update the branches of your repos.

* Change `config.organization` to your github username in `mungegithub/publisher/deployment/kubernetes/configmap.yaml`

* Deploy the publishing robot by running [deploy.sh](util/deploy.sh)

Then you can deploy the robot for real,

* Change `config.organization` to "kubernetes" in `mungegithub/publisher/deployment/kubernetes/configmap.yaml`

* Deploy the publishing robot by running [deploy.sh](util/deploy.sh)

## Known issues

1. Reporting issues: the publishing robot should file an issue and attach its logs if it meets bugs during publishing. 
2. Testing: currently we rely on manual testing. We should set up CI for it.
3. Automate release process (tracked at https://github.com/kubernetes/kubernetes/issues/49011): when kubernetes release, automatic update the configuration of the publishing robot.
