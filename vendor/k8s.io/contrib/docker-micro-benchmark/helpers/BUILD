load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "docker_helpers.go",
        "errors.go",
        "helpers.go",
    ],
    importpath = "k8s.io/contrib/docker-micro-benchmark/helpers",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/docker/docker/pkg/jsonmessage:go_default_library",
        "//vendor/github.com/docker/engine-api/client:go_default_library",
        "//vendor/github.com/docker/engine-api/types:go_default_library",
        "//vendor/github.com/docker/engine-api/types/container:go_default_library",
        "//vendor/github.com/docker/engine-api/types/network:go_default_library",
        "//vendor/github.com/docker/engine-api/types/strslice:go_default_library",
        "//vendor/github.com/juju/ratelimit:go_default_library",
        "//vendor/golang.org/x/net/context:go_default_library",
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
