load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "github.com/influxdata/influxdb/cmd/store",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/influxdata/influxdb/cmd:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/store/help:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/store/query:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb/engine:go_default_library",
        "//vendor/github.com/uber-go/zap:go_default_library",
    ],
)

go_binary(
    name = "store",
    embed = [":go_default_library"],
    importpath = "github.com/influxdata/influxdb/cmd/store",
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
        "//vendor/github.com/influxdata/influxdb/cmd/store/help:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/store/query:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
