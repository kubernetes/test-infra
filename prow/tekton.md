# Using Prow with Tekton Pipeline

The Prow agent type `tekton-pipeline` will allow you to trigger
execution of
[Tekton Pipelines](https://github.com/tektoncd/pipeline) 
with Prow!

1. [Setting up Pipelines with Prow](#setting-up-pipelines-with-prow)
2. [Configuring a `tekton-pipeline` job](#configuring-a-tekton-pipeline-job)

**Warnings**

* Since executing Pipelines cannot be decorated with
  [pod utilities](https://github.com/kubernetes/test-infra/blob/master/prow/pod-utilities.md),
  logs, metadata and artifacts will not be automatically uploaded to GCS by Prow.
* [`PipelineResources`](https://github.com/tektoncd/pipeline/blob/master/docs/resources.md)
  created by `Prow` are not garbage collected. (See
  [tektoncd/pipeline#544](https://github.com/tektoncd/pipeline/issues/544).)

## Setting up Pipelines with Prow

1. [Setup and configure Prow itself](getting_started_deploy.md)
2. [Install Tekton Pipelines](#install-tekton)
3. [Add the `pipeline` and `crier` controllers](#pipeline-and-crier-controllers)

### Install Tekton Pipelines

Before [the Prow pipeline controller](#pipeline-and-crier-controllers)
can start, and in order to be able to execute [Tekton Pipelines](https://github.com/tektoncd/pipeline,
you also need to install and setup Tekton Pipelines itself.

[Prow is currently compatible with versions 0.2.0 - 0.3.1 of Tekton Pipelines](https://github.com/kubernetes/test-infra/issues/13948)
so you must install one of those versions, e.g.:

```bash
kubectl apply --filename  https://storage.googleapis.com/tekton-releases/previous/v0.3.1/release.yaml
```

_The [configuration used above](#pipeline-and-crier-controllers)
for the Prow Pipeline controller assumes that Tekton Pipelines is
installed into its default namespace (`tekton-pipelines`)._

### `pipeline` and `crier` controllers

In addition to the services required by `Prow` for `prowjobs`, you will need
to add:

1. [The pipeline controller](/prow/cmd/pipeline): Creates
   [`PipelineRuns](https://github.com/tektoncd/pipeline/blob/v0.3.1/docs/pipelineruns.md)
2. [The crier controller](/prow/cmd/crier): Updates GitHub with the results of
   `ProwJobs` executed by agent types other than `kubernetes`

You can add these to your cluster with:

```bash
# Add the service account for the crier controller
k apply -f prow/cluster/crier_rbac.yaml

# Add the service account for the pipeline controller
k apply -f prow/cluster/pipeline_rbac.yaml

# Add the pipeline controller deployment
k apply -f prow/cmd/pipeline/simple_pipeline_deployment.yaml

# Add the crier controller deployment
k apply -f prow/cmd/crier/simple_crier_deployment.yaml

```

## Configuring a `tekton-pipeline` job

Once [you have everything setup](#setting-up-pipelines-with-prow) you can
configure Prow jobs to run Pipelines.

1. [Create and apply `Pipelines` and
   `Tasks`](#create-and-apply-pipelines-and-tasks)
2. [Configure Prow with `ProwJobs` that use those Pipelines and Tasks](#add-tekton-pipeline-prowjobws)
3. Then when the `ProwJob` is triggered:
  a. Prow (via [the Prow pipeline controller](#pipeline-and-crier-controllers))
     will create [a PipelineRun](https://github.com/tektoncd/pipeline/blob/v0.3.1/docs/pipelineruns.md)
  b. When the `PipelineRun` completes, the Prow Pipeline controller will update
     the `ProwJob`
  c. Finally, [crier](#pipeline-and-crier-controllers) will see the update to
     the `ProwJob` and update `GitHub` with the results

### Create and apply `Pipelines` and `Tasks`

Before anything can be executed, Tekton expects
[`Pipelines`](https://github.com/tektoncd/pipeline/blob/v0.3.1/docs/pipelines.md)
and the [`Tasks`](https://github.com/tektoncd/pipeline/blob/v0.3.1/docs/tasks.md)
that the `Pipelines` reference to exist.

Since [Prow is only compatible with versions 0.2.0 - 0.3.1 of Tekton Pipelines](https://github.com/kubernetes/test-infra/issues/13948)
the below docs are pinned to v0.3.1 of Tekton Pipelines:

* See [the Tekton tutorial](https://github.com/tektoncd/pipeline/blob/v0.3.1/docs/tutorial.md)
  for an overview
* See [the Tekton Pipelines docs](https://github.com/tektoncd/pipeline/tree/v0.3.1/docs#tekton-pipelines)
  for reference docs
* See [the Tekton catalog](https://github.com/tektoncd/catalog)
  for examples of `Tasks` you might want to use.

### Add `tekton-pipeline` ProwJobs

To configure a `ProwJob` to use Tekton Pipelines, include a `pipelineRunSpec`.

This is an example of a `presubmit` job that uses Tekton Pipelines:

```yaml
presubmits:
  bobcatfish/catservice:
  - name: do-the-pipeline
    agent: tekton-pipeline # Use the Prow Tekton controller!
    always_run: true
    pipeline_run_spec: # Template for creation of the `PipelineRun`
      trigger: # Required by versions 0.2.0 - v0.3.1 of Tekton Pipelines
        type: manual
      pipelineRef:
        name: special-pipeline # Use the Tekton Pipeline called `special-pipeline`
      resources:
      - name: git
        resourceRef:
          name: PROW_IMPLICIT_GIT_REF # Used by Prow to add the triggering git ref
```

* `agent: tekton-pipeline` - Tells Prow that execution of this job should be
  handed to [the Prow Pipeline controller](#pipeline-and-crier-controllers)
* `pipeline_run_spec` - The `spec` field of a
  [PipelineRun](https://github.com/tektoncd/pipeline/blob/v0.3.1/docs/pipelineruns.md),
  used to instantiate a new `PipelineRun` whenever triggered
* `trigger` - A
  [now deprecated field in a `PipelineRun`](https://github.com/tektoncd/pipeline/releases/tag/v0.4.0)
  which must be included though it does nothing (until
  [#13948](https://github.com/kubernetes/test-infra/issues/13948)!)
* `pipelineRef` - Refers to a
  [`Pipeline`](#create-and-apply-pipelines-and-tasks) that must exist for the
  `PipelineRun` to be able to execute
* `PROW_IMPLICIT_GIT_REF`/`PROW_EXTRA_GIT_REF_0` - Using this special
  [PipelineResource](https://github.com/tektoncd/pipeline/blob/v0.3.1/docs/resources.md)
  name results in Prow creating a new `PipelineResource` of type `git` which is
  instantiated with correct `url` and `revision` to be able to access the
  triggering branch. Using `PROW_EXTRA_GIT_REF_0`, `PROW_EXTRA_GIT_REF_1`, etc. allows
  Prow to create `PipelineResources` for any additional repos specified via `extra_refs`,
  like in the `periodic` example below.

This is an example of a `periodic` job that uses Tekton Pipelines:

```yaml
periodics:
- cron: "0 0 * * *"
  name: tekton-pipelines-nightly-release
  agent: tekton-pipeline # Use the Prow Tekton controller!
  extra_refs: # Specify the repo to create PipelineResources from
  - org: tektoncd
    repo: pipeline
    base_ref: master
  pipeline_run_spec:
    trigger: # Required by versions 0.2.0 - v0.3.1 of Tekton Pipelines
      type: manual
    pipelineRef:
      name: special-pipeline-nightly # Use the Tekton Pipeline called `special-pipeline-nightly`
    resources:
    - name: source-repo
      resourceRef:
        name: PROW_EXTRA_GIT_REF_0 # Used by Prow to add the correct git ref from `extra_refs`
```

For other fields (including how to specify `ServiceAccounts`) that can be set
for `PipelineRuns`, see
[the `PipelineRun` reference docs](https://github.com/tektoncd/pipeline/blob/v0.3.1/docs/pipelineruns.md).

_See [the `ProwJob` configuration docs](/prow/jobs.md)._
