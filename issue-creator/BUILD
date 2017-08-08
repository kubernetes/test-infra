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
    entrypoint = ["/issue-creator"],
    files = [
        ":issue-creator",
        ":test-owners-csv",
    ],
)

docker_bundle(
    name = "bundle",
    images = {
        "{STABLE_DOCKER_REPO}/issue-creator:{DOCKER_TAG}": ":image",
        "{STABLE_DOCKER_REPO}/issue-creator:latest": ":image",
        "{STABLE_DOCKER_REPO}/issue-creator:latest-{BUILD_USER}": ":image",
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
)

go_binary(
    name = "issue-creator",
    library = ":go_default_library",
    tags = ["automanaged"],
)

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    tags = ["automanaged"],
    deps = ["//issue-creator/creator:go_default_library"],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [
        ":package-srcs",
        "//issue-creator/creator:all-srcs",
        "//issue-creator/sources:all-srcs",
        "//issue-creator/testowner:all-srcs",
    ],
    tags = ["automanaged"],
)

filegroup(
    name = "test-owners-csv",
    srcs = [
        "test_owners.csv",
    ],
)
