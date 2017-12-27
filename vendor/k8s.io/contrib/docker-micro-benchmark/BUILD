load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "benchmark_plans.go",
        "config.go",
        "docker_micro_benchmark.go",
    ],
    importpath = "k8s.io/contrib/docker-micro-benchmark",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/docker/engine-api/client:go_default_library",
        "//vendor/k8s.io/contrib/docker-micro-benchmark/helpers:go_default_library",
    ],
)

go_binary(
    name = "docker-micro-benchmark",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/docker-micro-benchmark",
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
    srcs = [
        ":package-srcs",
        "//vendor/k8s.io/contrib/docker-micro-benchmark/helpers:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
