load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "errors.go",
        "influxdb.go",
        "node.go",
    ],
    importpath = "github.com/influxdata/influxdb",
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
        "//vendor/github.com/influxdata/influxdb/client:all-srcs",
        "//vendor/github.com/influxdata/influxdb/models:all-srcs",
        "//vendor/github.com/influxdata/influxdb/pkg/escape:all-srcs",
    ],
    tags = [
        "automanaged",
        "manual",
    ],
    visibility = ["//visibility:public"],
)
