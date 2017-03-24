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
* `cmd/horologium` starts periodic jobs when necessary.

## How to test prow

Build with:
```
bazel build //prow/...
```
Test with:
```
bazel test //prow/...
```

You can run `cmd/hook` in a local mode for testing, and hit it with arbitrary
fake webhooks. To do this, run in one shell:
```
./bazel-bin/prow/cmd/hook/hook --local --config-path prow/config.yaml --plugin-config prow/plugins.yaml
```
This will listen on `localhost:8888` for webhooks. Send one with:
```
./bazel-bin/prow/cmd/phony/phony --event issue_comment --payload prow/cmd/phony/examples/test_comment.json
```

## How to update the cluster

Any modifications to Go code will require redeploying the affected binaries.
Fortunately, this should result in no downtime for the system. Bump the
relevant version number in the makefile as well as in the `cluster` manifest,
then run the image and deployment make targets. For instance, if you bumped
the hook version, run `make hook-image && make hook-deployment`.

**Please ensure that your git tree is up to date before updating anything.**

## How to add new plugins

Add a new package under `plugins` with a method satisfying one of the handler
types in `plugins`. In that package's `init` function, call
`plugins.Register*Handler(name, handler)`. Then, in `cmd/hook/main.go`, add an
empty import so that your plugin is included. If you forget this step then a
unit test will fail when you try to add it to `plugins.yaml`. Don't add a brand
new plugin to the main `kubernetes/kubernetes` repo right away, start with
somewhere smaller and make sure it is well-behaved. If you add a command,
document it in [commands.md](../commands.md).

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

## How to turn up a new cluster

Prow should run anywhere that Kubernetes runs. Here are the steps required to
set up a prow cluster on GKE.

1. Create the cluster. I'm assuming that `PROJECT`, `CLUSTER`, and `ZONE` are
set. I'm putting prow components on a node with the label `role=prow`, and I'm
doing the actual tests on nodes with the label `role=build`, but this isn't a
hard requirement.

 ```
 gcloud -q container --project "${PROJECT}" clusters create "${CLUSTER}" --zone "${ZONE}" --machine-type n1-standard-4 --num-nodes 4 --node-labels=role=prow --scopes "https://www.googleapis.com/auth/compute","https://www.googleapis.com/auth/devstorage.full_control","https://www.googleapis.com/auth/logging.write","https://www.googleapis.com/auth/servicecontrol","https://www.googleapis.com/auth/service.management" --network "default" --enable-cloud-logging --enable-cloud-monitoring
 gcloud -q container node-pools create build-pool --project "${PROJECT}" --cluster "${CLUSTER}" --zone "${ZONE}" --machine-type n1-standard-8 --num-nodes 4 --local-ssd-count=1 --node-labels=role=build
 ```

1. Create the secrets that allow prow to talk to GitHub. The `hmac-token` is
the token that you set on GitHub webhooks, and the `oauth-token` is an OAuth2
token that has read and write access to the bot account.

 ```
 kubectl create secret generic hmac-token --from-file=hmac=/path/to/hook/secret
 kubectl create secret generic oauth-token --from-file=oauth=/path/to/oauth/secret
 ```

1. Create the secrets that allow prow to talk to Jenkins. The `jenkins-token`
is the API token that matches your Jenkins account. The `jenkins-address` is
Jenkins' URL, such as `http://pull-jenkins-master:8080`.

 ```
 kubectl create secret generic jenkins-token --from-file=jenkins=/path/to/jenkins/secret
 kubectl create configmap jenkins-address --from-file=jenkins-address=/path/to/address
 ```

1. Create the prow configs.

 ```
 kubectl create configmap config --from-file=config=config.yaml
 kubectl create configmap plugins --from-file=plugins=plugins.yaml
 ```

1. *Optional*: Create service account and SSH keys for your pods to run as.
This shouldn't be necessary for most use cases.

 ```
 kubectl create secret generic service-account --from-file=service-account.json=/path/to/service-account/secret
 kubectl create secret generic ssh-key-secret --from-file=ssh-private=/path/to/priv/secret --from-file=ssh-public=/path/to/pub/secret
 ```

1. Run the prow components that you desire. I recommend `hook`, `line`,
`sinker`, and `deck` to start out with. You'll need some way for ingress
traffic to reach your hook and deck deployments.

 ```
 make hook-image
 make line-image
 make sinker-image
 make deck-image

 make hook-deployment
 make hook-service
 make sinker-deployment
 make deck-deployment
 make deck-service

 kubectl apply -f cluster/ingress.yaml
 ```

1. Add the webhook to GitHub.
