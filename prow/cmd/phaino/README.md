# Phaino

Run prowjobs on your local workstation with `phaino`.

## Usage

Usage:
```console
# Use a job from deck
bazel run //prow/cmd/phaino -- $URL # or /path/to/prowjob.yaml
bazel run //prow/cmd/mkpj -- --config=/your/config --job=foo | bazel run //prow/cmd/phaino
```

### Common options

* `--grace=5m` controls how long to wait for interrupted jobs before terminating.
* `--print` the command that runs each job without running it
* `--privileged` jobs are allowed to run instead of rejected
* `--timeout=10m` controls how long to allow jobs to run before interrupting them

See `bazel run //prow/cmd/phaino -- --help` for full option list.


### Usage examples
URL example:

* Go to your [deck deployment](https://prow.k8s.io)
* Pick a job and click the rerun icon on the left
* Copy the URL (something like https://prow.k8s.io/rerun?prowjob=d08f1ca5-5d63-11e9-ab62-0a580a6c1281)
* Paste it as a phaino arg
  - `bazel run //prow/cmd/phaino -- https://prow.k8s.io/rerun?prowjob=d08f1ca5-5d63-11e9-ab62-0a580a6c1281
  - Alternatively `curl $URL | bazel run //prow/cmd/phaino`


A `mkpj` example:

* Use `mkpj` to create the job and pipe this to `phaino`
  - For prow.k8s.io jobs use `//config:mkpj`
      * `bazel run //config:mkpj -- --job=pull-test-infra-bazel | bazel run //prow/cmd/bazel`
  - Other deployments will need to clone that rule and/or pass in extra flags:
      * `bazel run //prow/cmd/mkpj -- --config=/my/config.yaml --job=my-job | bazel run //prow/cmd/bazel`

If you cannot use bazel (or do not want to), use `go get -u k8s.io/test-infra/prow/cmd/phaino`.
