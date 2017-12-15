load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "github.com/influxdata/influxdb/cmd/influx_inspect",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/influxdata/influxdb/cmd:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/dumptsi:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/dumptsm:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/export:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/help:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/inmem2tsi:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/report:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/verify:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb/engine:go_default_library",
    ],
)

go_binary(
    name = "influx_inspect",
    embed = [":go_default_library"],
    importpath = "github.com/influxdata/influxdb/cmd/influx_inspect",
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
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/dumptsi:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/dumptsm:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/export:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/help:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/inmem2tsi:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/report:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_inspect/verify:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
