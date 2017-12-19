load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "converter.go",
        "main.go",
        "tracker.go",
    ],
    importpath = "github.com/influxdata/influxdb/cmd/influx_tsm",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/influxdata/influxdb/cmd/influx_tsm/b1:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_tsm/bz1:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_tsm/stats:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_tsm/tsdb:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb/engine/tsm1:go_default_library",
    ],
)

go_binary(
    name = "influx_tsm",
    embed = [":go_default_library"],
    importpath = "github.com/influxdata/influxdb/cmd/influx_tsm",
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
        "//vendor/github.com/influxdata/influxdb/cmd/influx_tsm/b1:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_tsm/bz1:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_tsm/stats:all-srcs",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_tsm/tsdb:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
