# ProwJobs

For a brief overview of how Prow runs jobs take a look at ["Life of a Prow Job"](/prow/life_of_a_prow_job.md).

For a brief cookbook for jobs intended for [prow.k8s.io](https://prow.k8s.io/), please refer to
[`config/jobs/README.md`](/config/jobs/README.md)

Make sure Prow has been [deployed] correctly:

* The `horologium` component schedules periodic jobs.
* The `hook` component schedules presubmit and postsubmit jobs, ensuring the repo:
  - enabled `trigger` in [`plugins.yaml`]
  - sends GitHub webhooks to prow.
* The `plank` component schedules the pod requested by a prowjob.
* The `crier` component reports status back to github.

## How to configure new jobs

To configure a new job you'll need to add an entry into [config.yaml](/config/prow/config.yaml).
If you have [update-config](/prow/plugins/updateconfig) plugin deployed then the
config will be automatically updated once the PR is merged, else you will need
to run `make update-config`. This does not require redeploying any binaries,
and will take effect within a few minutes.

Alternatively, the [inrepoconfig](/prow/inrepoconfig.md) feature can be used to version Presubmit jobs
in the same repository that also contains the code and have Prow load them dynamically.
See [its documentation](/prow/inrepoconfig.md) for more details.

Prow requires you to have a basic understanding of kubernetes, such
that you can define pods in yaml.  Please see kubernetes documentation
for help here, for example the [Pod overview] and [PodSpec api
reference].

Periodic config looks like so:

```yaml
periodics:
- name: foo-job         # Names need not be unique, but must match the regex ^[A-Za-z0-9-._]+$
  decorate: true        # Enable Pod Utility decoration. (see below)
  interval: 1h          # Anything that can be parsed by time.ParseDuration.
  # Alternatively use a cron instead of an interval, for example:
  # cron: "05 15 * * 1-5"  # Run at 7:05 PST (15:05 UTC) every M-F
  spec: {}              # Valid Kubernetes PodSpec.
```

Postsubmit config looks like so:

```yaml
postsubmits:
  org/repo:
  - name: bar-job         # As for periodics.
    decorate: true        # As for periodics.
    spec: {}              # As for periodics.
    max_concurrency: 10   # Run no more than this number concurrently.
    branches:             # Regexps, only run against these branches.
    - ^master$
    skip_branches:        # Regexps, do not run against these branches.
    - ^release-.*$
```

Postsubmits are run when a push event happens on a repo, hence they are
configured per-repo. If no `branches` are specified, then they will run against
every branch.

Presubmit config looks like so:

```yaml
presubmits:
  org/repo:
  - name: qux-job            # As for periodics.
    decorate: true           # As for periodics.
    always_run: true         # Run for every PR, or only when requested.
    run_if_changed: "qux/.*" # Regexp, only run on certain changed files.
    skip_report: true        # Whether to skip setting a status on GitHub.
    context: qux-job         # Status context. Defaults to the job name.
    max_concurrency: 10      # As for postsubmits.
    spec: {}                 # As for periodics.
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
command that reruns all jobs. If unspecified, the default configuration makes
`/test <job-name>` trigger the job.

## Presets

[`Presets`] can be used to define commonly reused values for a subset of fields
for PodSpecs and BuildSpecs. The subset of fields chosen was inspired by
[PodPresets] which at time of writing are still in alpha. A preset config looks
like:

```yaml
presets:
- labels:                  # a job with these labels/values will have the preset applied
    preset-foo-bar: "true" #   key:value pair must be unique among presets
  env:                     # list of valid Kubernetes EnvVars
  - name: FOO
    value: BAR
  volumes:                 # list of valid Kubernetes Volumes
  - name: foo
    emptyDir: {}
  - name: bar
    secret:
      secretName: bar
  volumeMounts:            # list of valid Kubernetes VolumeMounts
  - name: foo
    mountPath: /etc/foo
  - name: bar
    mountPath: /etc/bar
    readOnly: true
- env:                     # a preset with no labels is applied to all jobs
  - name: BAZ
    value: qux
  volumes:
    # etc...
  volumeMounts:
    # etc...
```

## Standard Triggering and Execution Behavior for Jobs

When configuring jobs, it is necessary to keep in mind the set of rules Prow has
for triggering jobs, the GitHub status contexts that those jobs provide, and the
rules for protecting those contexts on branches.

### Triggering Jobs
#### Trigger Types

`prow` will consider three different types of jobs that run on pull requests
(presubmits):

 1. jobs that run unconditionally and automatically. All jobs that set
     `always_run: true` fall into this set.
 2. jobs that run conditionally, but automatically. All jobs that set
    `run_if_changed` to some value fall into this set.
 3. jobs that run conditionally, but not automatically. All jobs that set
    `always_run: false` and do not set `run_if_changed` to any value fall
    into this set and require a human to trigger them with a command.

By default, jobs fall into the third category and must have their `always_run` or
`run_if_changed` configured to operate differently.

In the rest of this document, "a job running unconditionally" indicates that the
job will run even if it is normally conditional and the conditions are not met.
Similarly, "a job running conditionally" indicates that the job runs if all of its
conditions are met.

#### Triggering Jobs With Comments

A developer may trigger presubmits by posting a comment to a pull request that
contains one or more of the following phrases:

 - `/test job-name` : When posting `/test job-name`, any jobs with matching triggers
   will be triggered unconditionally.
 - `/retest` : When posting `/retest`, two types of jobs will be triggered:
   - all jobs that have run and failed will run unconditionally
   - any not-yet-executed automatically run jobs will run conditionally
 - `/test all` : When posting `/test all`, all automatically run jobs will run
   conditionally.

Note: It is possible to configure a job's `trigger` to match any of the above keywords
(`/retest` and/or `/test all`) but this behavior is not suggested as it will confuse
developers that expect consistent behavior from these commands. More generally, it is
possible to configure a job's `trigger` to match any command that is otherwise known
to Prow in some other context, like `/close`. It is similarly not suggested to do this.

#### Posting GitHub Status Contexts

Presubmit and postsubmit jobs post a status context to the GitHub
commit under test once they start, unless the job is configured
with `skip_report: true`.

Use a `/retest` or `/test job-name` to re-trigger the test and
hopefully update the failed context to passing.

If a job should no longer trigger on the pull request, use the
`/skip` command to dismiss a failing status context (depends on
`skip` plugin).

Repo administrators can also `/override job-name` in case of emergency
(depends on the `override` plugin).

### Requiring Job Statuses
#### Requiring Jobs for Auto-Merge Through Tide

Tide will treat jobs in the following manner for merging:

 - unconditionally run jobs with required status contexts are always required to have
   passed on a pull request to merge
 - conditionally run jobs with required status contexts are required to have passed on
   a pull request to merge if the job currently matches the pull request.
 - jobs with optional status contexts are ignored when merging

In order to set a job's context to be optional, set `optional: true` on the job. If it
is required to not post the results of the job to GitHub whatsoever, the job may be set
to be optional and silent by setting `skip_report: true`. It is valid to set both of
these options at the same time.

#### Protecting Status Contexts

The branch protection rules will only enforce the presence of jobs that run unconditionally
and have required status contexts. As conditionally-run jobs may or may not post a status
context to GitHub, they cannot be required through this mechanism.

## Pod Utilities

If you are adding a new job that will execute on a Kubernetes cluster (`agent: kubernetes`, the default value) you should consider using the [Pod Utilities](/prow/pod-utilities.md). The pod utils decorate jobs with additional containers that transparently provide source code checkout and log/metadata/artifact uploading to GCS.

## Job Environment Variables

Prow will expose the following environment variables to your job. If the job
runs on Kubernetes, the variables will be injected into every container in
your pod, If the job is run in Jenkins, Prow will supply them as parameters to
the build.

Variable | Periodic | Postsubmit | Batch | Presubmit | Description | Example
--- |:---:|:---:|:---:|:---:| --- | ---
`CI` | ✓ | ✓ | ✓ | ✓ | Represents whether the current environment is a CI environment | `true`
`ARTIFACTS` | ✓ | ✓ | ✓ | ✓ | Directory in which to place files to be uploaded when the job completes | `/logs/artifacts`
`JOB_NAME` | ✓ | ✓ | ✓ | ✓ | Name of the job. | `pull-test-infra-bazel`
`JOB_TYPE` | ✓ | ✓ | ✓ | ✓ | Type of job. | `presubmit`
`JOB_SPEC` | ✓ | ✓ | ✓ | ✓ | JSON-encoded job specification. | see below
`BUILD_ID` | ✓ | ✓ | ✓ | ✓ | Unique build number for each run. | `12345`
`PROW_JOB_ID` | ✓ | ✓ | ✓ | ✓ | Unique identifier for the owning Prow Job. | `1ce07fa2-0831-11e8-b07e-0a58ac101036`
`REPO_OWNER` | | ✓ | ✓ | ✓ | GitHub org that triggered the job. | `kubernetes`
`REPO_NAME` | | ✓ | ✓ | ✓ | GitHub repo that triggered the job. | `test-infra`
`PULL_BASE_REF` | | ✓ | ✓ | ✓ | Ref name of the base branch. | `master`
`PULL_BASE_SHA` | | ✓ | ✓ | ✓ | Git SHA of the base branch. | `123abc`
`PULL_REFS` | | ✓ | ✓ | ✓ | All refs to test. | `master:123abc,5:qwe456`
`PULL_NUMBER` | | | | ✓ | Pull request number. | `5`
`PULL_PULL_SHA` | | | | ✓ | Pull request head SHA. | `qwe456`

Examples of the JSON-encoded job specification follow for the different
job types:

Periodic Job:
```json
{"type":"periodic","job":"job-name","buildid":"0","prowjobid":"uuid","refs":{}}
```

Postsubmit Job:
```json
{"type":"postsubmit","job":"job-name","buildid":"0","prowjobid":"uuid","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha"}}
```

Presubmit Job:
```json
{"type":"presubmit","job":"job-name","buildid":"0","prowjobid":"uuid","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"}]}}
```

Batch Job:
```json
{"type":"batch","job":"job-name","buildid":"0","prowjobid":"uuid","refs":{"org":"org-name","repo":"repo-name","base_ref":"base-ref","base_sha":"base-sha","pulls":[{"number":1,"author":"author-name","sha":"pull-sha"},{"number":2,"author":"other-author-name","sha":"second-pull-sha"}]}}
```

## Testing a new job

See ["How to test a ProwJob"](/prow/build_test_update.md#How-to-test-a-ProwJob).

## Badges

Prow can display badges that signal whether jobs are passing ([example](https://prow.k8s.io/badge.svg?jobs=post-test-infra-bazel)).

The format to send your `deck` URL is `/badge.svg?jobs=single-job-name` or `/badge.svg?jobs=common-job-prefix-*`.

<!-- links -->

[Pod overview]: https://kubernetes.io/docs/concepts/workloads/pods/pod-overview/#pod-templates
[PodPresets]: https://kubernetes.io/docs/concepts/workloads/pods/podpreset/
[PodSpec api reference]: https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.16/#podspec-v1-core
[`Presets`]: https://github.com/kubernetes/test-infra/blob/3afb608d28630b99e49e09dd101a96c201268739/prow/config/jobs.go#L33-L40
[`plugins.yaml`]: /config/prow/plugins.yaml
[deployed]: https://github.com/kubernetes/test-infra/blob/master/prow/getting_started_deploy.md
