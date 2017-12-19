load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["converters.go"],
    importpath = "github.com/influxdata/influxdb/prometheus",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/influxdata/influxdb/models:go_default_library",
        "//vendor/github.com/influxdata/influxdb/prometheus/remote:go_default_library",
        "//vendor/github.com/influxdata/influxql:go_default_library",
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
    srcs = [
        ":package-srcs",
        "//vendor/github.com/influxdata/influxdb/prometheus/remote:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
