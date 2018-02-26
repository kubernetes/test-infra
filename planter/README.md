# Planter 
Bazel in a container.

<img src="planter-logo.svg" />

Planter is a container + wrapper script for your bazel builds.
It will run a Docker container as the current user that can run bazel builds
in your `$PWD`. It has been tested on macOS and Linux against
`kubernetes/test-infra` and `kubernetes/kubernetes`.

To build kubernetes set up your `$GOPATH/src` to contain:
```
$GOPATH/src/k8s.io/kubernetes/   # ... <kubernetes/kubernetes checkout>
$GOPATH/src/k8s.io/test-infra/   # ... <kubernetes/test-infra checkout>
```
Then from `$GOPATH/src/k8s.io/kubernetes/` run:
 `./../planter/planter.sh make bazel-build`.

 For `test-infra` you can run eg `./planter/planter.sh bazel test //...`.

## Options

Planter respects the following environment variables:

 - `TAG`: The Planter image tag. This will default to the current stable
   version used to build Kubernetes, but you may override it with EG
   `TAG=0.9.0 ./planter.sh bazel build //...`
   - These should now match bazel release versions eg `0.8.0rc2`
 - `DRY_RUN`: If set, Planter will only echo the Docker command that would have
   been run.
 - `HOME`: Your home directory. This will be mounted in to the container.
 - `PWD`: Will be set to the working directory in the image.
 - `USER`: Used to run the image as the current user.

## SELinux

Currently, SELinux is disabled for the container that runs the bazel
environment, which allows for the rest of the host system to leave SELinux
enabled. Automatic relabeling is not done to avoid inadvertently causing issues
with the host system.


Further details can be found in `planter.sh` itself, which is somewhat
self-documenting.
