# prow-base image

Prow-base image is a dockerfile that is used by all pure Golang prow binaries.

## Usage

```
docker \
    build \
    -t gcr.io/k8s-prow/<BINARY_NAME> \
    --build-arg IMAGE_NAME=<BINARY_NAME> \
    -f \
    ./images/prow-base/Dockerfile \
    .
```

`<BINARY_NAME>` above needs to be replaced by real value. For example, to build `prow-controller-manager`, the command is:

```
docker \
    build \
    -t gcr.io/k8s-prow/prow-controller-manager \
    --build-arg IMAGE_NAME=prow-controller-manager \
    -f \
    ./images/prow-base/Dockerfile \
    .
```
