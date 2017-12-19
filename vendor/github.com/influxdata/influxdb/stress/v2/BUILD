load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "github.com/influxdata/influxdb/stress/v2",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/influxdata/influxdb/client/v2:go_default_library",
        "//vendor/github.com/influxdata/influxdb/stress/v2/stress_client:go_default_library",
        "//vendor/github.com/influxdata/influxdb/stress/v2/stressql:go_default_library",
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
        "//vendor/github.com/influxdata/influxdb/stress/v2/statement:all-srcs",
        "//vendor/github.com/influxdata/influxdb/stress/v2/stress_client:all-srcs",
        "//vendor/github.com/influxdata/influxdb/stress/v2/stressql:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
