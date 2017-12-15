load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "github.com/influxdata/influxdb/cmd/influx",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/influxdata/influxdb/client:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/influx/cli:go_default_library",
    ],
)

go_binary(
    name = "influx",
    embed = [":go_default_library"],
    importpath = "github.com/influxdata/influxdb/cmd/influx",
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
        "//vendor/github.com/influxdata/influxdb/cmd/influx/cli:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
