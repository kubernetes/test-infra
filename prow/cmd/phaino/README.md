# Phaino

Run prowjobs on your local workstation with `phaino`.

Plato believed that [ideas and forms] are the ultimate truth,
whereas we only see the imperfect physical appearances of those idea.

He linkens this in his [Allegory of the Cave] to someone living in a cave
who can only see the shadows projected on the wall
from objects passing in front of a fire.

[Phaino] is act of making those imperfect shadows appear.

Phaino shares a prefix with [Pharos], meaning lighthouse and in particular the ancient one in Alexandria.

## Usage

Usage:
```console
# Use a job from deck
bazel run //prow/cmd/phaino -- $URL # or /path/to/prowjob.yaml
# Use mkpj to create the job
bazel run //prow/cmd/mkpj -- --config-path=/path/to/prow/config.yaml --job-config-path=/path/to/prow/job/configs --job=foo > /tmp/foo
bazel run //prow/cmd/phaino -- /tmp/foo
```

Phaino is an interactive utility; it will prompt you for a local copy of any secrets or
volumes that the Prow Job may require.

### Common options

* `--grace=5m` controls how long to wait for interrupted jobs before terminating
* `--print` the command that runs each job without running it
* `--privileged` jobs are allowed to run instead of rejected
* `--timeout=10m` controls how long to allow jobs to run before interrupting them
* `--repo=k8s/test-infra=/go/src/k8s-test-infra` local path where prow job required repos are cloned at, can be passed in repeatedly

See `bazel run //prow/cmd/phaino -- --help` for full option list.


### Usage examples
#### URL example

* Go to your [deck deployment](https://prow.k8s.io)
* Pick a job and click the rerun icon on the left
* Copy the URL (something like https://prow.k8s.io/rerun?prowjob=d08f1ca5-5d63-11e9-ab62-0a580a6c1281)
* Paste it as a phaino arg
  - `bazel run //prow/cmd/phaino -- https://prow.k8s.io/rerun?prowjob=d08f1ca5-5d63-11e9-ab62-0a580a6c1281`
  - Alternatively `bazel run //prow/cmd/phaino -- <(curl $URL)`


#### Configuration example:

* Use [`mkpj`](/prow/cmd/mkpj) to create the job and pipe this to `phaino`
  - For prow.k8s.io jobs use `//config:mkpj`
      ```
      bazel run //config:mkpj -- --job=pull-test-infra-bazel > /tmp/foo
      bazel run //prow/cmd/phaino -- /tmp/foo
      ```
  - Other deployments will need to clone that rule and/or pass in extra flags:
      ```
      bazel run //prow/cmd/mkpj -- --config-path=/my/config.yaml --job=my-job
      bazel run //prow/cmd/phaino -- /tmp/foo
      ```

If you cannot use bazel (or do not want to), use `go get -u k8s.io/test-infra/prow/cmd/phaino`.


[ideas and forms]: https://en.wikipedia.org/wiki/Theory_of_forms#Forms
[Allegory of the Cave]: https://en.wikipedia.org/wiki/Allegory_of_the_Cave
[Phaino]: https://en.wiktionary.org/wiki/%CF%86%CE%B1%CE%AF%CE%BD%CF%89
[Pharos]: https://en.wikipedia.org/wiki/Lighthouse_of_Alexandria
