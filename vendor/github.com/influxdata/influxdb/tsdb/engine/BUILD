load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["engine.go"],
    importpath = "github.com/influxdata/influxdb/tsdb/engine",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/influxdata/influxdb/tsdb/engine/tsm1:go_default_library"],
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
        "//vendor/github.com/influxdata/influxdb/tsdb/engine/tsm1:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
