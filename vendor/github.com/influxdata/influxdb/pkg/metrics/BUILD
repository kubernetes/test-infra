load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "context.go",
        "counter.go",
        "default_registry.go",
        "descriptors.go",
        "doc.go",
        "group.go",
        "group_registry.go",
        "registry.go",
        "timer.go",
    ],
    importpath = "github.com/influxdata/influxdb/pkg/metrics",
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
