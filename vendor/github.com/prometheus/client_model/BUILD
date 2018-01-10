load("@io_bazel_rules_go//go:def.bzl", "go_library")
load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")

proto_library(
    name = "io_prometheus_client_proto",
    srcs = ["metrics.proto"],
    tags = ["manual"],
    visibility = ["//visibility:public"],
)

go_proto_library(
    name = "io_prometheus_client_go_proto",
    importpath = "github.com/prometheus/client_model",
    proto = ":io_prometheus_client_proto",
    tags = ["manual"],
    visibility = ["//visibility:public"],
)

go_library(
    name = "go_default_library",
    embed = [":io_prometheus_client_go_proto"],
    importpath = "github.com/prometheus/client_model",
    tags = ["manual"],
    visibility = ["//visibility:public"],
)

filegroup(
    name = "package-srcs",
    srcs = glob(["**"]),
    tags = [
        "automanaged",
        "manual",
    ],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [
        ":package-srcs",
        "//vendor/github.com/prometheus/client_model/go:all-srcs",
    ],
    tags = [
        "automanaged",
        "manual",
    ],
    visibility = ["//visibility:public"],
)
