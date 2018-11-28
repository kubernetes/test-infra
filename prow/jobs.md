# ProwJobs

For a brief overview of how Prow runs jobs take a look at ["Life of a Prow Job"](/prow/life_of_a_prow_job.md).

## How to configure new jobs

To configure a new job you'll need to add an entry into [config.yaml](config.yaml).
If you have [update-config](plugins/updateconfig) plugin deployed then the
config will be automatically updated once the PR is merged, else you will need
to run `make update-config`. This does not require redeploying any binaries,
and will take effect within a minute.

Periodic config looks like so:

```yaml
periodics:
- name: foo-job         # Names need not be unique, but must match the regex ^[A-Za-z0-9-._]+$
  decorate: true        # Enable Pod Utility decoration. (see below)
  interval: 1h          # Anything that can be parsed by time.ParseDuration.
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
    branches:             # Only run against these branches.
    - master
    skip_branches:        # Do not run against these branches.
    - release
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

### Pod Utilities

If you are adding a new job that will execute on a Kubernetes cluster (`agent: kubernetes`, the default value) you should consider using the [Pod Utilities](/prow/pod-utilities.md). The pod utils decorate jobs with additional containers that transparently provide source code checkout and log/metadata/artifact uploading to GCS.


## Job Environment Variables

Prow will expose the following environment variables to your job. If the job
runs on Kubernetes, the variables will be injected into every container in
your pod, If the job is run in Jenkins, Prow will supply them as parameters to
the build.

Variable | Periodic | Postsubmit | Batch | Presubmit | Description | Example
--- |:---:|:---:|:---:|:---:| --- | ---
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
