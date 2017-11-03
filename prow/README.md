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

See also: [Life of a Prow Job](./architecture.md) 

## Announcements

Breaking changes to external APIs (labels, GitHub interactions, configuration
or deployment) will be documented in this section. Prow is in a pre-release
state and no claims of backwards compatibility are made for any external API.
Note: versions specified in these announcements may not include bug fixes made
in more recent versions so it is recommended that the most recent versions are
used when updating deployments.

 - *November 3, 2017* Added `EmptyDir` volume type. To update to `hook:0.176+`
   or `horologium:0.11+` the following components must have the associated
   minimum versions: `deck:0.58+`, `plank:0.54+`, `jenkins-operator:0.50+`.
 - *November 2, 2017* `plank:0.53` changes the `type` label key to `prow.k8s.io/type`
   and the `job` annotation key to `prow.k8s.io/job` added in pods.
 - *October 13, 2017* `hook:0.174`, `plank:0.50`, and `jenkins-operator:0.47`
   drop the deprecated `github-bot-name` flag.
 - *October 2, 2017* `hook` version 0.171. The label plugin was split into three
   plugins (label, sigmention, milestonestatus). Breaking changes:
   - The configuration key for the milestone maintainer team's ID has been
   changed. Previously the team ID was stored in the plugins config at key
   `label`>>`milestone_maintainers_id`. Now that the milestone status labels are
   handled in the `milestonestatus` plugin instead of the `label` plugin, the
   team ID is stored at key `milestonestatus`>>`maintainers_id`.
   - The sigmention and milestonestatus plugins must be enabled on any repos
   that require them since their functionality is no longer included in the 
   label plugin.
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

## Getting started

[See the doc here.](./getting_started.md)

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
`plugins.Register*Handler(name, handler)`. Then, in `hook/plugins.go`, add an
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

Note that Github events triggered by the account that is managing the plugins
are ignored by some plugins. It is prudent to use a different bot account for
performing merges or rerunning tests, whether the deployment that drives the
second account is `tide` or the `submit-queue` munger.

## How to add new jobs

To add a new job you'll need to add an entry into [config.yaml](config.yaml). 
If you have [update-config](plugins/updateconfig) plugin deployed then the 
config will be automatically updated once the PR is merged, else you will need 
to run `make update-config`. This does not require redeploying any binaries, 
and will take effect within a minute.

Periodic config looks like so:

```yaml
periodics:
- name: foo-job         # Names need not be unique.
  interval: 1h          # Anything that can be parsed by time.ParseDuration.
  agent: kubernetes     # See discussion.
  spec: {}              # Valid Kubernetes PodSpec.
  run_after_success: [] # List of periodics.
```

The `agent` should be "kubernetes", but if you are running a controller for a
different job agent then you can fill that in here. The spec should be a valid
Kubernetes PodSpec iff `agent` is "kubernetes".

Postsubmit config looks like so:

```yaml
postsubmits:
  org/repo:
  - name: bar-job         # As for periodics.
    agent: kubernetes     # As for periodics.
    spec: {}              # As for periodics.
    max_concurrency: 10   # Run no more than this number concurrently.
    branches:             # Only run against these branches.
    - master
    skip_branches:        # Do not run against these branches.
    - release
    run_after_success: [] # List of postsubmits.
```

Postsubmits are run when a push event happens on a repo, hence they are
configured per-repo. If no `branches` are specified, then they will run against
every branch.

Presubmit config looks like so:

```yaml
presubmits:
  org/repo:
  - name: qux-job            # As for periodics.
    always_run: true         # Run for every PR, or only when requested.
    run_if_changed: "qux/.*" # Regexp, only run on certain changed files.
    skip_report: true        # Whether to skip setting a status on GitHub.
    context: qux-job         # Status context. Usually the job name.
    max_concurrency: 10      # As for postsubmits.
    agent: kubernetes        # As for periodics.
    spec: {}                 # As for periodics.
    run_after_success: []    # As for periodics.
    branches: []             # As for postsubmits.
    skip_branches: []        # As for postsubmits.
    trigger: "(?m)qux test this( please)?" # Regexp, see discussion.
    rerun_command: "qux test this please"  # String, see discussion.
```

If you only want to run tests when specific files are touched, you can use
`run_if_changed`. A useful pattern when adding new jobs is to start with
`always_run` set to false and `skip_report` set to true. Test it out a few
times by manually triggering, then switch `always_run` to true. Watch for a
couple days, then switch `skip_report` to false.

The `trigger` is a regexp that matches the `rerun_command`. Users will be told
to input the `rerun_command` when they want to rerun the job. Actually, anything
that matches `trigger` will suffice. This is useful if you want to make one
command that reruns all jobs.


### Job Evironment Variables

Prow will expose the following environment variables to your job. If the job
runs on Kubernetes, the variables will be injected into every container in
your pod, If the job is run in Jenkins, Prow will supply them as parameters to
the build.

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

Note: to not overwrite the Jenkins `$BUILD_NUMBER` variable, the build identifier
will be passed as `$buildId` to Jenkins jobs.

## Bots home

[@k8s-ci-robot](https://github.com/k8s-ci-robot) and its silent counterpart
[@k8s-bot](https://github.com/k8s-bot) both live here as triggers to GitHub
messages defined in [config.yaml](config.yaml). Here is a
[command list](https://github.com/kubernetes/test-infra/blob/master/commands.md)
for them. 
