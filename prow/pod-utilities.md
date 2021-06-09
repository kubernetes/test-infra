# Pod Utilities

Pod utilities are small, focused Go programs used by `plank` to decorate user-provided `PodSpec`s
in order to increase the ease of integration for new jobs into the entire CI infrastructure. The
utilities today wrap the execution of the test code to ensure that the tests run against correct
versions of the source code, that test commands run in the appropriate environment and that output
from the test (in the form of status, logs and artifacts) is correctly uploaded to the cloud.

These utilities are integrated into a test run by adding `InitContainer`s and sidecar `Container`s
to the user-provided `PodSpec`, as well as by overwriting the `Container` entrypoint for the test
`Container` provided by the user. The following utilities exist today:

 - [`clonerefs`](./cmd/clonerefs/README.md): clones source code under test
 - [`initupload`](./cmd/initupload/README.md): records the beginning of a test in cloud storage
   and reports the status of the clone operations
 - [`entrypoint`](./cmd/entrypoint/README.md): is injected into the test `Container`, wraps the
   test code to capture logs and exit status
 - [`sidecar`](./cmd/sidecar/README.md): runs alongside the test `Container`, uploads status, logs
   and test artifacts to cloud storage once the test is finished

### Why use Pod Utilities?

Writing a ProwJob that uses the Pod Utilities is much easier than writing one
that doesn't because the Pod Utilities will transparently handle many of the
tasks the job would otherwise need to do in order to prepare its environment
and output more than pass/fail. Historically, this was achieved by wrapping
every job with a [bootstrap.py](/jenkins/bootstrap.py) script that handled cloning
source code, preparing the test environment, and uploading job metadata, logs,
and artifacts. This was cumbersome to configure and required every job to be
wrapped with the script in the job image. The pod utilities achieve the same goals
with less configuration and much simpler job images that are easier to develop
and less coupled to Prow.

## Writing a ProwJob that uses Pod Utilities

### What the test container can expect

Example test container script:
```bash
pwd # my repo root
ls path/to/file/in/my/repo.txt # access repo file
ls ../other-repo # access repo file in another repo
echo success > ${ARTIFACTS}/results.txt # result info that will be uploaded to GCS.
# logs, and job metadata are automatically uploaded.
```

More specifically, a ProwJob using the Pod Utilities can expect the following:
- **Source Code** - Jobs can expect to begin execution with their working
directory set as the root of the checked out repo. The commit that is checked
out depends on the type of job:
	- `presubmit` jobs will have the relevant PR checked out and merged with the base branch.
	- `postsubmit` jobs will have the upstream commit that triggered the job checked out.
	- `periodic` jobs will have the working directory set to the root of the repo specified by the first ref in `extra_refs` (if specified).
See the `extra_refs` field if you need to clone more than one repo.
- **Metadata and Logs** - Jobs can expect metadata about the job to be uploaded
before the job starts, and additional metadata and logs to be uploaded when the
job completes.
- **Artifact Directory** - Jobs can expect an `$ARTIFACTS` environment variable
to be specified. It indicates an existent directory where job artifacts can be
dumped for automatic upload to GCS upon job completion.

### How to configure

In order to use the pod utilities, you will need to configure plank with some settings first.
See plank's [README](/prow/plank) for reference.

ProwJobs may request Pod Utility decoration by setting `decorate: true` in their config.
Example ProwJob configuration:
```yaml

  - name: pull-job
    always_run: true
    decorate: true
    spec:
      containers:
      - image: alpine
        command:
        - "echo"
        args:
        - "The artifacts dir is $(ARTIFACTS)"
```

In addition to normal ProwJob configuration, ProwJobs using the Pod Utilities
must specify the `command` field in the container specification instead of using
the Dockerfile's ENTRYPOINT directive. Note that the `command` field is a string
array not just a string. It should point to the test binary location in the container.

Additional fields may be required for some use cases:
- Private repos need to do two things:
	- Add an ssh secret that gives the bot access to the repo to the build cluster
	and specify the secret name in the `ssh_key_secrets` field of the job decoration config.
	- Set the `clone_uri` field of the job spec to `git@github.com:{{.Org}}/{{.Repo}}.git`.
- Repos requiring a non-standard clone path can use the `path_alias` field
to clone the repo to different go import path than the default of `/home/prow/go/src/github.com/{{.Org}}/{{.Repo}}/` (e.g. `path_alias: k8s.io/test-infra` -> `/home/prow/go/src/k8s.io/test-infra`).
- Jobs that require additional repos to be checked out can arrange for that with
the `exta_refs` field. If the cloned path of this repo must be used as a default working dir the `workdir: true` must be specified.
- Jobs that do not want submodules to be cloned should set `skip_submodules` to `true`
- Jobs that want to perform shallow cloning can use `clone_depth` field. It can be set to desired clone depth. By default, clone_depth get set to 0 which results in full clone of repo.

```yaml
- name: post-job
  decorate: true
  decoration_config:
    ssh_key_secrets:
    - ssh-secret
  clone_uri: "git@github.com:<YOUR_ORG>/<YOUR_REPO>.git"
  extra_refs:
  - org: kubernetes
    repo: other-repo
    base_ref: master
    workdir: false
  skip_submodules: true
  clone_depth: 0
  spec:
    containers:
    - image: alpine
      command:
      - "echo"
      args:
      - "The artifacts dir is $(ARTIFACTS)"

```

### Migrating from bootstrap.py to Pod Utilities

Jobs using the deprecated [bootstrap.py](/jenkins/bootstrap.py) should switch to the Pod Utilities at
their earliest convenience. @dims has created a handy [migration guide](https://gist.github.com/dims/c1296f8ed42238baea0a5fcae45f4cf4).

## Automatic Censoring of Secret Data

Many jobs exist that must touch third-party systems in order to be productive. Whether the job provisions
resources in a cloud IaaS like GCP, reports results to an aggregation service like coveralls.io, or simply
clones private repositories, jobs require sensitive credentials to achieve their goals. Even with the best
intentions, it is possible for end-user code running in a test `Pod` for a `ProwJob` to accidentally leak
the content of those credentials. If Prow is configured to push job logs and artifacts to a public cloud
storage bucket, that leak is immediately immortalized in plain text for the world to read. The `sidecar`
utility can infer what secrets a job has access to and censor those secrets from the output. The following
job turns on censoring:

```yaml
- name: censored-job
  decorate: true
  decoration_config:
    censor_secrets: true
```

### Censoring Process

The automatic censoring process is written to be as useful as possible while having a bounded impact on the
execution cost in resources and time for the job. In order to censor every possible leak, all keys in all
`Secrets` that are mounted into the test `Pod` are treated as sensitive data. For each of these keys, the
value of the key as well as the base-64 encoded value are censored from the job's log as well as any
artifacts the job produces. If any archives (e.g. `.tar.gz`) are found in the output artifacts for a job,
they are unarchived in order to censor their contents on the fly before being re-archived and pushed up to
cloud storage.

In order to bound the impact in runtime and resource cost for censoring on the job, both the concurrency
and buffer size of the censoring algorithm are tunable. The overall steady-state memory footprint of the
censoring algorithm is simply the buffer size times the maximum concurrency. The buffer must be as large
as twice the length of the largest secret to be censored, but may be tuned to very small values in order
to decrease the memory footprint. Keep mind that this will increase overall disk I/O and therefore increase
the runtime of censoring. Therefore, in order to decrease censoring runtime the buffer should be increased.

### Configuring Censoring

A number of aspects of the censoring algorithm are tunable with configuration option at the per-job level
or for entire repositories or organizations. Under the `decoration_config` stanza, the following options
are available to tune censoring:

```yaml
decoration_config:
  censoring_options:
    censoring_concurrency: 0 # the number of files to censor concurrently; each allocates a buffer
    censoring_buffer_size: 0 # the size of the censoring buffer, in bytes
    include_directories:
    - path/**/to/*something.txt # globs relative to $ARTIFACTS that should be censored; everything censored if unset
    exclude_directories:
    - path/**/to/*other.txt # globs relative to $ARTIFACTS that should not be censored
```
