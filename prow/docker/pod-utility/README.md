## TL;DR

In order to run prow jobs on platforms other than amd64, we need to build the following four images:

1. clonerefs
2. initupload
3. entrypoint
4. sidecar

The source code can be found at https://github.com/kubernetes/test-infra/tree/master/prow/cmd

## Build pod utility images manually

### Retrieve and build bazel for ppc64le and s390x

Since the [test-infra](https://github.com/kubernetes/test-infra) repo uses [bazel](https://bazel.build/) as build tool and bazel doesn't provide ppc64le and s390x binary officially. so we firstly have to retrieve and build **bazel** for power and Z according to the community-supported doc:

- ppc64le: retrieve the binary from [community-supported site](https://oplab9.parqtec.unicamp.br/pub/ppc64el/bazel/)
- s390x: build with the [community-supported doc](https://github.com/linux-on-ibm-z/docs/wiki/Building-Bazel)

**Note:** Actually the bazel version for this doc is too old, while the [test-infra](https://github.com/kubernetes/test-infra) repo requires least bazel **>= 1.2.1**. If you encounter `zip -d` issue while building bazel-v1.2.1, you can workaround that with: https://github.com/bazelbuild/bazel/pull/10798

### How to build pod utility binaries

After the bazel binaries on ppc64le and s390x are successfully built, we can start to the four pod utility binaries for prow job with the following commands:

```
bazel build prow/cmd/clonerefs
bazel build prow/cmd/initupload
bazel build prow/cmd/entrypoint
bazel build prow/cmd/sidecar
```

**Note:** If you encounter issue of `cannot execute binary file: Exec format error` of `nodejs`, you have to replace it with s390x `nodejs` in bazel cache, because the [test-infra](https://github.com/kubernetes/test-infra) repo uses hard-coded x86 nodejs binary.

After all the four binaries are built successfully, you can verify they are working by testing the binaries locally on build machine to make sure the binaries are runnable on corresponding platforms.

### How to build pod utility images

As we mentioned before that the [test-infra](https://github.com/kubernetes/test-infra) repo uses [bazel](https://bazel.build/) as build tool and bazel makes full use of [rules_docker](https://github.com/bazelbuild/rules_docker) to make these pod utility images. Unfortunately, the [rules_docker](https://github.com/bazelbuild/rules_docker) can't be migrated easily to ppc64le and s390x. Therefore, to build pod utility images on these platforms, we can compose the equal dockerfiles to build docker images traditionally.

- [Dockfile.clonerefs](./Dockfile.clonerefs)
- [Dockfile.entrypoint](./Dockfile.entrypoint)
- [Dockfile.initupload](./Dockfile.initupload)
- [Dockfile.sidecar](./Dockfile.sidecar)

## Build pod utility images automatically

You can also build the pod utility binaries and images with [create_utility_images.sh](./create_utility_images.sh) automatically.

## Issues

- We encountered coredump issue when runnning the pod utility images, it was caused by the golang version, try to build with latest golang will resolve that.
