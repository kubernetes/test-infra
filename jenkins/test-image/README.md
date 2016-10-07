# Test Images

From Kubernetes 1.4, we are making one test-images per Kubernetes version.


# Naming:
User can add more Dockerfile and name it to Dockerfile-[VERSION].

[VERSION] will be used as part of the built image tag.


User can build one more all images by specify VERSIONS value, which is all Dockerfile-* by default.
for example, `make push VERSIONS=1.4` will build Dockerfile-1.4 and tag it and push it to k8s-testimages.

To build multiple images, you can do something like `make build VERSIONS='1.4 1.5'`

For more details, run make help, or see "Usage" below.

# Usage:
`make all:        make build`

`make build:      builds all the Dockerfile-* in VERSIONS`

`make push:       builds all the Dockerfile-* in VERSIONS and pushes to k8s-testimages`

