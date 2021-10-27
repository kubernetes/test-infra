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
* `--code-mount-path=/go` changes the path where code is mounted in the container
* `--skip-volume-mounts=volume1,volume2` includes the unwanted volume mounts that are defined in the job spec
* `--extra-volume-mounts=/go/src/k8s.io/test-infra=/Users/xyz/k8s-test-infra` includes the extra volume mounts needed for the container. Key is the mount path and value is the local path
* `--skip-envs=env1,env2` includes the unwanted env vars that are defined in the job spec
* `--extra-envs=env1=val1,env2=val2` includes the extra env vars needed for the container
* `--use-local-gcloud-credentials` controls whether to use the same gcloud credentials as local or not
* `--use-local-kubeconfig` controls whether to use the same kubeconfig as local or not

#### Common options usage scenarios

Phaino is smart at prompting for where repo is located, volume mounts etc., if
it's desired to save the prompts, use the following tricks instead:

- If the repo needs to be cloned under GOPATH, use:
  ```
  --code-mount-path==/whatever/go/src # Controls where source code is mounted in container
  --extra-volume-mounts=/whatever/go/src/k8s.io/test-infra=/Users/xyz/k8s-test-infra
  ```
- If job requires mounting kubeconfig, assume the mount is named `kubeconfig`,use:
  ```
  --use-local-kubeconfig
  --skip-volume-mounts=kubeconfig
  ```
- If job requires mounting gcloud default credentials, assume the mount is named `service-account`,use:
  ```
  --use-local-gcloud-credentials
  --skip-volume-mounts=service-account
  ```
- If job requires mounting something else like `name:foo; mountPath: /bar`,use:
  ```
  --extra-volume-mounts=/bar=/Users/xyz/local/bar
  --skip-volume-mounts=foo
  ```
- If job requires env vars,use:
  ```
  --extra-envs=env1=val1,env2=val2
  ```


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
