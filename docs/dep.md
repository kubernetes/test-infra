# kubernetes/test-infra dependency management

test-infra uses [`dep`](https://github.com/golang/dep) for Go dependency
management. `dep` is a prototype dependency management tool for Go. It requires
Go 1.8 or newer to compile.


## Setup

You can follow the [setup instructions](https://github.com/golang/dep#setup) to
set up `dep` in your local environment.

test-infra is currently using dep 0.4.1. This version has also been vendored
into the repository. You can run the vendored version using `bazel run`; for
example, run
```console
$ bazel run //:dep -- ensure -add example.org/foo/bar
```
to add a new dependency.

## Changing dependencies

You can use the `dep` instructions for
[adding](https://golang.github.io/dep/docs/daily-dep.html#adding-a-new-dependency),
[updating](https://golang.github.io/dep/docs/daily-dep.html#updating-dependencies) or
[removing](https://golang.github.io/dep/docs/daily-dep.html#using-dep-ensure) a dependency.

Once you've updated, make sure to run:
```
hack/update-deps.sh
```
This will generate necessary Bazel BUILD files and will also remove any
unused libraries that dep fails to prune.

## Tips

If `dep ensure` doesn't come back and freezes, please make sure `hg` command is
installed on your environment. `dep ensure` requires `hg` command for getting
bitbucket.org/ww/goautoneg , but `dep ensure` doesn't output such error message
and just freezes. [reference](https://github.com/kubernetes/test-infra/issues/5987)
