load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["parse.go"],
    importpath = "github.com/influxdata/influxdb/cmd",
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
        "//vendor/github.com/influxdata/influxdb/cmd/influx:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_stress:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_tsm:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/influxd:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
