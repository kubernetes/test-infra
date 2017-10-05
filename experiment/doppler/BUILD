package(default_visibility = ["//visibility:public"])

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
    entrypoint = ["/doppler"],
    files = [":doppler"],
)

docker_bundle(
    name = "bundle",
    images = {
        "{STABLE_DOCKER_REPO}/doppler:{DOCKER_TAG}": ":image",
        "{STABLE_DOCKER_REPO}/doppler:latest": ":image",
        "{STABLE_DOCKER_REPO}/doppler:latest-{BUILD_USER}": ":image",
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

go_library(
    name = "go_default_library",
    srcs = [
        "main.go",
        "metrics.go",
    ],
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/prometheus/client_golang/prometheus:go_default_library",
        "//vendor/github.com/prometheus/client_golang/prometheus/promhttp:go_default_library",
    ],
)

go_binary(
    name = "doppler",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
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
