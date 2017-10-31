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

## Options

Planter repects the following environment variables:

 - `TAG`: the planter image tag, this will default to the current stable version
 used to build kubernetes, but you may override it with EG `TAG=0.6.1-1`
 - `DRY_RUN`: if set planter will only echo the docker command that would have been run

 - `HOME`: your home directory, this will be mounted in to the container
 - `PWD`: will be set to the working directory in the image
 - `USER`: used to run the image as the current user

