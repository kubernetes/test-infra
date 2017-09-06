# Prow

Prow is the system that handles GitHub events and commands for Kubernetes. It
currently comprises several related pieces that live in a Kubernetes cluster.
See the [GoDoc](https://godoc.org/k8s.io/test-infra/prow) for library docs.
Please note that these libraries are intended for use by prow only, and we do
not make any attempt to preserve backwards compatibility.

* `cmd/hook` is the most important piece. It is a stateless server that listens
  for GitHub webhooks and dispatches them to the appropriate handlers.
* `cmd/plank` is the controller that manages jobs running in k8s pods.
* `cmd/jenkins-operator` is the controller that manages jobs running in Jenkins.
* `cmd/sinker` cleans up old jobs and pods.
* `cmd/splice` regularly schedules batch jobs.
* `cmd/deck` presents [a nice view](https://prow.k8s.io/) of recent jobs.
* `cmd/phony` sends fake webhooks.
* `cmd/tot` vends incrementing build numbers.
* `cmd/horologium` starts periodic jobs when necessary.
* `cmd/mkpj` creates `ProwJobs`.

See also: [Life of a Prow Job](https://github.com/kubernetes/test-infra/blob/master/prow/architecture.md).

## Announcements

Breaking changes to external APIs (labels, GitHub interactions, configuration
or deployment) will be documented in this section. Prow is in a pre-release
state and no claims of backwards compatibility are made for any external API.

 - *September 3, 2017* sinker:0.17 now deletes pods labeled by plank:0.42 in
   order to avoid cleaning up unrelated pods that happen to be found in the
   same namespace prow runs pods. If you run other pods in the same namespace,
   you will have to manually delete or label the prow-owned pods, otherwise you
   can bulk-label all of them with the following command and let sinker collect
   them normally:
   ```
   kubectl label pods --all -n pod_namespace created-by-prow=true
   ```
 - *September 1, 2017* `deck` version 0.44 and `jenkins-operator` version 0.41
   controllers no longer provide a default value for the `--jenkins-token-file` flag.
   Cluster administrators should provide `--jenkins-token-file=/etc/jenkins/jenkins`
   explicitly when upgrading to a new version of these components if they were
   previously relying on the default. For more context, please see
   [this pull request.](https://github.com/kubernetes/test-infra/pull/4210)
 - *August 29, 2017* Configuration specific to plugins is now held in in the
   `plugins` `ConfigMap` and serialized in this repo in the `plugins.yaml` file.
   Cluster administrators upgrading to `hook` version 0.148 or newer should move
   plugin configuration from the main `ConfigMap`. For more context, please see
   [this pull request.](https://github.com/kubernetes/test-infra/pull/4213)

## How to test prow

Build with:
```
bazel build //prow/...
```
Test with:
```
bazel test --features=race //prow/...
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

## How to run a given job on prow

Run the following, specifying `JOB_NAME`:

```
bazel run //prow/cmd/mkpj -- --job=JOB_NAME
```

This will print the ProwJob YAML to stdout. You may pipe it into `kubectl`.
Depending on the job, you will need to specify more information such as PR
number.

## How to update the cluster

Any modifications to Go code will require redeploying the affected binaries.
Fortunately, this should result in no downtime for the system. Run `./bump.sh <program-name>` 
to bump the relevant version number in the makefile as well as in the `cluster` manifest,
then run the image and deployment make targets on a branch which has the 
changes. For instance, if you bumped the hook version, run 
`make hook-image && make hook-deployment`.

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

Add an entry to [plugins.yaml](plugins.yaml). If you misspell the name then a 
unit test will fail. If you have [update-config](plugins/updateconfig) plugin 
deployed then the config will be automatically updated once the PR is merged, 
else you will need to run `make update-plugins`. This does not require 
redeploying the binaries, and will take effect within a minute.

## How to add new jobs

To add a new job you'll need to add an entry into [config.yaml](config.yaml). 
If you have [update-config](plugins/updateconfig) plugin deployed then the 
config will be automatically updated once the PR is merged, else you will need 
to run `make update-config`. This does not require redeploying any binaries, 
and will take effect within a minute.

Prow will inject the following environment variables into every container in
your pod:

Variable | Periodic | Postsubmit | Batch | Presubmit | Description | Example
--- |:---:|:---:|:---:|:---:| --- | ---
`JOB_NAME` | ✓ | ✓ | ✓ | ✓ | Name of the job. | `pull-test-infra-bazel`
`BUILD_NUMBER` | ✓ | ✓ | ✓ | ✓ | Unique build number for each run. | `12345`
`REPO_OWNER` | | ✓ | ✓ | ✓ | GitHub org that triggered the job. | `kubernetes`
`REPO_NAME` | | ✓ | ✓ | ✓ | GitHub repo that triggered the job. | `test-infra`
`PULL_BASE_REF` | | ✓ | ✓ | ✓ | Ref name of the base branch. | `master`
`PULL_BASE_SHA` | | ✓ | ✓ | ✓ | Git SHA of the base branch. | `123abc`
`PULL_REFS` | | ✓ | ✓ | ✓ | All refs to test. | `master:123abc,5:qwe456`
`PULL_NUMBER` | | | | ✓ | Pull request number. | `5`
`PULL_PULL_SHA` | | | | ✓ | Pull request head SHA. | `qwe456`

## Bots home

[@k8s-ci-robot](https://github.com/k8s-ci-robot) and its silent counterpart
[@k8s-bot](https://github.com/k8s-bot) both live here as triggers to GitHub
messages defined in [config.yaml](config.yaml). Here is a
[command list](https://github.com/kubernetes/test-infra/blob/master/commands.md)
for them. 

## How to turn up a new cluster

Prow should run anywhere that Kubernetes runs. Here are the steps required to
set up a prow cluster on GKE.

1. Create the cluster. I'm assuming that `PROJECT`, `CLUSTER`, and `ZONE` are
set. You can also choose to run the builds in a separate cluster.

 ```
 gcloud -q container --project "${PROJECT}" clusters create "${CLUSTER}" --zone "${ZONE}" --machine-type n1-standard-4 --num-nodes 4 --scopes "https://www.googleapis.com/auth/compute","https://www.googleapis.com/auth/devstorage.full_control","https://www.googleapis.com/auth/logging.write","https://www.googleapis.com/auth/servicecontrol","https://www.googleapis.com/auth/service.management" --network "default" --enable-cloud-logging --enable-cloud-monitoring
 gcloud -q container node-pools create build-pool --project "${PROJECT}" --cluster "${CLUSTER}" --zone "${ZONE}" --machine-type n1-standard-8 --num-nodes 4 --local-ssd-count=1
 ```

2. Create the secrets that allow prow to talk to GitHub. The `hmac-token` is
the token that you set on GitHub webhooks, and the `oauth-token` is an OAuth2
token that has read and write access to the bot account.

 ```
 kubectl create secret generic hmac-token --from-file=hmac=/path/to/hook/secret
 kubectl create secret generic oauth-token --from-file=oauth=/path/to/oauth/secret
 ```

3. Create the secrets that allow prow to talk to Jenkins. The `jenkins-token`
is the API token that matches your Jenkins account. The `jenkins-address` is
Jenkins' URL, such as `http://pull-jenkins-master:8080`.

 ```
 kubectl create secret generic jenkins-token --from-file=jenkins=/path/to/jenkins/secret
 kubectl create configmap jenkins-address --from-file=jenkins-address=/path/to/address
 ```

4. Create the prow configs.

 ```
 kubectl create configmap config --from-file=config=config.yaml
 kubectl create configmap plugins --from-file=plugins=plugins.yaml
 ```

5. Create the ProwJob ThirdPartyResource.

 ```
 kubectl create -f cluster/prow_job.yaml
 ```

6. Create a namespace for test pods named "test-pods". Its name must match the
value of `pod_namespace` in [config.yaml](config.yaml). 

 ```
 kubectl create namespace test-pods
 ```

7. *Optional*: Create service account and SSH keys for your pods to run as.
This shouldn't be necessary for most use cases. They'll need to be in the same
namespace as the pods.

 ```
 kubectl create secret generic service-account --namespace=test-pods --from-file=service-account.json=/path/to/service-account/secret
 kubectl create secret generic ssh-key-secret --namespace=test-pods --from-file=ssh-private=/path/to/priv/secret --from-file=ssh-public=/path/to/pub/secret
 ```

8. Run the prow components that you desire. I recommend `hook`, `plank`,
`sinker`, and `deck` to start out with. You'll need some way for ingress
traffic to reach your hook and deck deployments.

 ```
 make hook-image
 make plank-image
 make sinker-image
 make deck-image

 make hook-deployment
 make hook-service
 make plank-deployment
 make sinker-deployment
 make deck-deployment
 make deck-service

 kubectl apply -f cluster/ingress.yaml
 ```

9. Add the webhook to GitHub.

Hook processes the following events: issues, issue_comment, pull_request, push, status so github web hooks should be configured to send these event types. We suggest configuring your webhooks to send everything so that future event types are covered.

The content-type for all webhooks type must be application/json.  This is selectable via a dropdown in the webhook configuration page.
