load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "inmem.go",
        "meta.go",
    ],
    importpath = "github.com/influxdata/influxdb/tsdb/index/inmem",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/influxdata/influxdb/models:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/bytesutil:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/escape:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/estimator:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/estimator/hll:go_default_library",
        "//vendor/github.com/influxdata/influxdb/query:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb:go_default_library",
        "//vendor/github.com/influxdata/influxql:go_default_library",
        "//vendor/github.com/uber-go/zap:go_default_library",
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
