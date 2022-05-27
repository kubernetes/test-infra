# Fake Git Server (FGS)

FGS is actually not a fake at all. It is a real web server that serves real Git
repositories them over HTTP. FGS wraps around the vanilla `git http-backend`
subcommand that comes with Git, calling it as a CGI executable. It supports both
read (e.g., `git clone`, `git fetch`) and write (e.g., `git push`) operations
against it.

FGS is used for integration tests. See `TestClonerefs` for an example.

## Usage in Integration Testing

The `fakegitserver.go` file is built automatically by `hack/prowimagebuilder`,
and we deploy it to the KIND cluster. Inside the cluster, it accepts web traffic
at the endpoint `http://fakegitserver.default` (`http://localhost/fakegitserver`
from outside of the KIND cluster).

There are 2 routes:

- `/repo/<REPO_NAME>`: endpoint for Git clients to interact (`git clone`, `git fetch`,
  `git push`). E.g., `git clone http://fakegitserver.default/repo/foo`.
  Internally, FGS serves all Git repo folders under `-git-repos-parent-dir` on
  disk and serves them for the `/repo` route with the `git-http-backend` CGI
  script.
- POST `/setup-repo`: endpoint for creating new Git repos on the server; you
  just need to send a JSON payload like this:

```json
{
  "name": "foo",
  "overwrite": true,
  "script": "echo hello world > README; git add README; git commit -m update"
}
```

Here is a cURL example:

```shell
# mkFoo is a plaintext file containing the JSON from above.
$ curl http://localhost/fakegitserver/setup-repo -d @mkFoo
commit c1e4e5bb8ba0e5b16147450a75347a27e5980222
Author: abc <d@e.f>
Date:   Thu May 19 12:34:56 2022 +0000

    update


```

Notice how the server responds with a `git log` output of the just-created repo
to ease debugging in case repos are not created the way you expect them to be
created.

During integration tests, each test creates repo(s) using the `/setup-repo`
endpoint as above. Care must be taken to not reuse the same repository name, as
the test cases (e.g., the test cases in `TestClonerefs`) all run in parallel and
can clobber each other's repo creation setp.

### Allowing Push Access

Although this is not (yet) used in tests, push access is enabled for all served
repos. This is achieved by setting the `http.receivepack` Git configuration
option to `true` for each repo found under `-git-repos-parent-dir`. This is
because the `git http-backend` script does not by default allow anonymous push
access unless the aforementioned option is set to `true` on a per-repo basis.

### Allowing Fetching of Commit SHAs

By default the CGI script will only serve references that are "advertised" (such
as those references under `refs/heads/*` or `refs/pull/*/head`). However, FGS
also sets the `uploadpack.allowAnySHA1InWant` option to `true` to allow Git
clients (such as clonerefs) to fetch commits by their SHA.

## Local Usage (for debugging)

FGS has 2 requirements:

1. the path to the local `git` binary installation, and
2. the path to a folder containing Git repositories to be served (can be an
   empty directory, or pre-populated).

By default port 8888 is used, although this can also be configured with `-port`.

Example:

```shell
$ go run fakegitserver.go -h
Usage of /tmp/go-build2317700172/b001/exe/fakegitserver:
  -git-binary git
        Path to the git binary. (default "/usr/bin/git")
  -git-repos-parent-dir string
        Path to the parent folder containing all Git repos to serve over HTTP. (default "/git-repo")
  -port int
        Port to listen on. (default 8888)

$ go run fakegitserver.go -git-repos-parent-dir <PATH_TO_REPOS> -git-binary <PATH_TO_GIT>
{"component":"unset","file":"/home/someuser/go/src/k8s.io/test-infra/prow/test/integration/fakegitserver/fakegitserver.go:111","func":"main.main","level":"info","msg":"Start server","severity":"info","time":"2022-05-22T20:31:38-07:00"}
```

In this example, `http://localhost:8888` is the HTTP address of FGS:

```shell
# Clone "foo" repo, assuming it exists locally under `-git-repos-parent-dir`.
$ git clone http://localhost:8888/repo/foo
$ cd foo
$ git log # or any other arbitrary Git command
# ... do some Git operations
$ git push
```

That's it!

## Local Usage with Docker and Ko (for debugging)

It may be helpful to run FGS in a containerized environment for debugging. First
install [ko](https://github.com/google/ko#Kubernetes-Integration) itself. Then
`cd` to the `fakegitserver` folder (same folder as this README.md file), and
run:

```shell
# First CD to the root of the repo, because the .ko.yaml configuration (unfortunately)
# depends on relative paths that can only work from the root of the repo.
$ cd ${PATH_TO_REPO_ROOT}
$ docker run -it --entrypoint=sh -p 8123:8888 $(ko build --local k8s.io/test-infra/prow/test/integration/fakegitserver)
```

The `-p 8123:8888` allows you to talk to the containerized instance of
fakegitserver over port 8123 on the host.

### Custom Base Image

To use a custom base image for FGS, change the `baseImageOverrides` entry for
fakegitserver in [`.ko.yaml`](/.ko.yaml) like this:

```yaml
baseImageOverrides:
  # ... other entries ...
  k8s.io/test-infra/prow/test/integration/fakegitserver: gcr.io/my/base/image:tag
```

If you want `ko` to pick up a local Docker image on your machine, rename the
image to have a `ko.local` prefix. For example, like this:

```yaml
baseImageOverrides:
  k8s.io/test-infra/prow/test/integration/fakegitserver: ko.local/my/base/image:tag
```
