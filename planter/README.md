# Planter

Planter is a container + wrapper script for your bazel builds.
It will run a docker container as the current user that can run bazel builds
in your `$PWD`. It has been tested on macOS and Linux against 
`kubernetes/test-infra` and `kubernetes/kubernetes`.

To build kubernetes set up your `$GOPATH/src` to contain:
```
$GOPATH/src/k8s.io/kubernetes/ ... <kubernetes/kubernetes checkout>
$GOPATH/src/k8s.io/test-infra/ ... <kubernetes/test-infra checkout>
```
Then from `$GOPATH/src/k8s.io/kubernetes/` run:
 `./../planter/planter.sh make bazel-build`.

 For `test-infra` you can run eg `./planter/planter.sh bazel test //...`.
