package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_docker//docker:docker.bzl",
    "docker_build",
    "docker_bundle",
)
load(
    "@io_bazel_rules_docker//docker/contrib:push-all.bzl",
    "docker_push",
)

docker_build(
    name = "image",
    base = "@distroless-base//image",
    entrypoint = ["/commenter"],
    files = [":commenter"],
)

docker_bundle(
    name = "bundle",
    images = {
        "{STABLE_DOCKER_REPO}/commenter:{DOCKER_TAG}": ":image",
        "{STABLE_DOCKER_REPO}/commenter:latest": ":image",
        "{STABLE_DOCKER_REPO}/commenter:latest-{BUILD_USER}": ":image",
    },
    stamp = True,
)

docker_push(
    name = "push",
    bundle = ":bundle",
)

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
    "go_test",
)

go_binary(
    name = "commenter",
    library = ":go_default_library",
    tags = ["automanaged"],
)

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    tags = ["automanaged"],
    deps = ["//prow/github:go_default_library"],
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
)

go_test(
    name = "go_default_test",
    srcs = ["main_test.go"],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = ["//prow/github:go_default_library"],
)
