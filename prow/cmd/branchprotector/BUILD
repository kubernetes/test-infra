# Usage:
#   bazel run :dev-job.{create,delete,describe}
load("@io_bazel_rules_k8s//k8s:object.bzl", "k8s_object")

k8s_object(
    name = "oneshot",
    cluster = "{STABLE_PROW_CLUSTER}",  # Run make :get-cluster-credentials
    image_chroot = "{STABLE_DOCKER_REPO}",
    images = {
        "gcr.io/k8s-testimages/branchprotector:latest": ":branchprotector-image",
    },
    kind = "Job",
    template = ":oneshot-job.yaml",
)

load("@io_bazel_rules_docker//docker:docker.bzl", "docker_bundle")
load("@io_bazel_rules_docker//contrib:push-all.bzl", "docker_push")
load("@io_bazel_rules_docker//go:image.bzl", "go_image")

docker_bundle(
    name = "bundle",
    images = {
        "{STABLE_PROW_REPO}/branchprotector:{DOCKER_TAG}": ":branchprotector-image",
        "{STABLE_PROW_REPO}/branchprotector:latest": ":branchprotector-image",
        "{STABLE_PROW_REPO}/branchprotector:latest-{BUILD_USER}": ":branchprotector-image",
    },
    stamp = True,
)

docker_push(
    name = "push",
    bundle = ":bundle",
)

go_image(
    name = "branchprotector-image",
    binary = ":branchprotector",
)

load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["protect.go"],
    importpath = "k8s.io/test-infra/prow/cmd/branchprotector",
    visibility = ["//visibility:public"],
    deps = [
        "//prow/config:go_default_library",
        "//prow/github:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = [
        "protect_test.go",
    ],
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/prow/cmd/branchprotector",
    deps = [
        "//prow/config:go_default_library",
        "//prow/github:go_default_library",
    ],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)

go_binary(
    name = "branchprotector",
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/prow/cmd/branchprotector",
    visibility = ["//visibility:public"],
)
