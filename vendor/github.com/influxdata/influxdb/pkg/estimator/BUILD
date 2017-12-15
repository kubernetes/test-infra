load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["sketch.go"],
    importpath = "github.com/influxdata/influxdb/pkg/estimator",
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
        "//vendor/github.com/influxdata/influxdb/pkg/estimator/hll:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
