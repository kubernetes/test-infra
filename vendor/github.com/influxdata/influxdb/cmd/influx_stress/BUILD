load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["influx_stress.go"],
    importpath = "github.com/influxdata/influxdb/cmd/influx_stress",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/influxdata/influxdb/stress:go_default_library",
        "//vendor/github.com/influxdata/influxdb/stress/v2:go_default_library",
    ],
)

go_binary(
    name = "influx_stress",
    embed = [":go_default_library"],
    importpath = "github.com/influxdata/influxdb/cmd/influx_stress",
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
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
