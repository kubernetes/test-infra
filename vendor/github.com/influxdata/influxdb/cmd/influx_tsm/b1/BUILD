load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["reader.go"],
    importpath = "github.com/influxdata/influxdb/cmd/influx_tsm/b1",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/boltdb/bolt:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_tsm/stats:go_default_library",
        "//vendor/github.com/influxdata/influxdb/cmd/influx_tsm/tsdb:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb/engine/tsm1:go_default_library",
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
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
