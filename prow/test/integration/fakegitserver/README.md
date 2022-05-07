# Fake Git Server (FGS)

FGS is actually not a fake at all. It is a real web server that serves real Git
repositories them over HTTP. FGS wraps around the vanilla `git http-backend`
subcommand that comes with Git, calling it as a CGI executable. It supports both
read (e.g., `git clone`, `git fetch`) and write (e.g., `git push`) operations
against it.

## Usage in Integration Testing

The `fakegitserver.go` file is built automatically by `hack/prowimagebuilder`.

## Local Usage

FGS needs has 2 requirements:

1. the path to the local `git` binary installation
2. the path to a folder containing Git repositories to be served.

By default port 8888 is used, although this can also be configured with `-port`.

Example:

```shell
$ go run fakegitserver.go -h
Usage of /tmp/go-build2317700172/b001/exe/fakegitserver:
  -foo-repo-remote-URL string
        URL of foo repo, as a submodule from inside the bar repo. This is only used when -populate-sample-repos is given. (default "http://localhost:8888/repo/foo")
  -git-binary git
        Path to the git binary. (default "/usr/bin/git")
  -git-repos-parent-dir string
        Path to the parent folder containing all Git repos to serve over HTTP. (default "/git-repo")
  -populate-sample-repos
        Whether to populate /git-repo with hardcoded sample repos. Used for integration tests.
  -port int
        Port to listen on. (default 8888)

$ go run fakegitserver.go -git-repos-parent-dir <PATH_TO_REPOS> -git-binary <PATH_TO_GIT>
{"component":"unset","file":"/home/someuser/go/src/k8s.io/test-infra/prow/test/integration/fakegitserver/fakegitserver.go:111","func":"main.main","level":"info","msg":"Start server","severity":"info","time":"2022-05-22T20:31:38-07:00"}
```

In this example, `http://localhost:8888` is the HTTP address of FGS:

```shell
$ git clone http://localhost:8888/foo.git
$ cd foo.git
$ git log # or any other arbitrary Git command
# ... do some Git operations
$ git push
```

That's it!

## Local Usage with Docker and Ko

It may be helpful to run FGS in a containerized environment for debugging. First
install [ko](https://github.com/google/ko#Kubernetes-Integration) itself. Then
`cd` to the `fakegitserver` folder (where this README.md is located), and run:

```shell
cd ${PATH_TO_REPO_ROOT}
$ docker run -it --entrypoint=sh -p 8123:8888 $(ko build --local k8s.io/test-infra/prow/test/integration/fakegitserver)
```

The `-p 8123:8888` allows you to talk to the containerized instance of
fakegitserver over port 8123 on the host.

### Custom Base Image

To use a custom base image for FGS, change the `baseImageOverrides` entry for
fakegitserver in [`.ko.yaml`](../../../../.ko.yaml) like this:

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

## Internals

FGS uses the [go-git](https://github.com/go-git/go-git) library to perform Git
tasks, except those that are not supported by the library.

When FGS starts up, by default it populates sample reproducible repositories
under `-git-repos-parent-dir`. These repos (by default `/git-repo/foo.git` and
`/git-repo/bar.git`) have some commits in them, as well as GitHub-style refs for
Pull Requests. These refs are then fetched and merged by clonerefs during
integration tests.

In addition, FGS looks at all Git repo folders it can find underneath
`-git-repos-parent-dir` and serves them as well. This is mostly a side effect of
letting the `git-http-backend` CGI script look at the `-git-repos-parent-dir`
folder.

### Allowing Push Access

Although this is not (yet) used in tests, push access is enabled for all served
repos. This is achieved by setting the `http.receivepack` Git configuration
option to `true` for each repo found under `-git-repos-parent-dir`. This is
because the `git http-backend` script does not by default allow anonymous push
access unless the aforementioned option is set to `true` on a per-repo basis.

### Allowing Fetching of Commit SHAs

By default the CGI script will only serve references that are "advertised" (such
as those references under `refs/heads/*` or `refs/pull/*/head`). However, FGS
also sets the `uploadpack.allowAnySHA1InWant` option to `true` to allow
clonerefs to fetch commits by their SHA.
