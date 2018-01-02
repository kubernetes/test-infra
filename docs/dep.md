# kubernetes/test-infra dependency management

test-infra uses [`dep`](https://github.com/golang/dep) for Go dependency
management. `dep` is a prototype dependency management tool for Go. It requires
Go 1.8 or newer to compile.


## Setup

You can follow the [setup instructions](https://github.com/golang/dep#setup) to
set up `dep` in your local environment.


## Changing dependencies

You can use the `dep` instructions for [adding](https://github.com/golang/dep#adding-a-dependency),
[updating](https://github.com/golang/dep#updating-dependencies) or
[removing](https://github.com/golang/dep#removing-dependencies) a dependency.

Once you've updated, make sure to run:
```
dep prune
hack/update-bazel.sh
```

To prune unneeded deps, and then update all the bazel files that `dep` blows away.

## Tips

If `dep ensure` doesn't come back and freezes, please make sure `hg` command is
installed on your environment. `dep ensure` requires `hg` command for getting
bitbucket.org/ww/goautoneg , but `dep ensure` doesn't output such error message
and just freezes. [reference](https://github.com/kubernetes/test-infra/issues/5987)
