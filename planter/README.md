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

NOTE: if you previously built Kubernetes by other means, you may need to run
`make clean` first to clean up some symlink cycles.

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
enabled. We could relabel the volumes and enable SELinux but this could cause
major issues on the host if planter was used from say $HOME.


Further details can be found in `planter.sh` itself, which is somewhat
self-documenting.

## Docker for Mac

Performance with docker for mac can be quite bad compared to installing bazel 
natively (which is an option!). If you are going to use planter though, 
consider tuning the following Docker options:

- Increase CPU reservation, 4+ cores recommended
- Increase Memory reservation, 8+ GB recommended

You can find these under [preferences > advanced](https://docs.docker.com/docker-for-mac/#advanced)

Check [this unofficial guide](https://medium.com/@TomKeur/how-get-better-disk-performance-in-docker-for-mac-2ba1244b5b70)
and make sure that you are using `.raw` formatted VM disk for the daemon. 

Periodically restarting the daemon (docker for mac tray icon > restart) can
also help. In particular if you see the Bazel analysis phase taking a long time
consider restarting the docker daemon before trying again.

We also use [delegated volume mounts](https://docs.docker.com/docker-for-mac/osxfs-caching/) to improve osxfs performance.

