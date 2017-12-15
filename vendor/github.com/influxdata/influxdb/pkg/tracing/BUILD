load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "context.go",
        "doc.go",
        "rawspan.go",
        "span.go",
        "spancontext.go",
        "trace.go",
        "trace_encoding.go",
        "tree.go",
        "util.go",
    ],
    importpath = "github.com/influxdata/influxdb/pkg/tracing",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/gogo/protobuf/proto:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/tracing/fields:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/tracing/labels:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/tracing/wire:go_default_library",
        "//vendor/github.com/xlab/treeprint:go_default_library",
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
        "//vendor/github.com/influxdata/influxdb/pkg/tracing/fields:all-srcs",
        "//vendor/github.com/influxdata/influxdb/pkg/tracing/labels:all-srcs",
        "//vendor/github.com/influxdata/influxdb/pkg/tracing/wire:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
