load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "batcher.go",
        "config.go",
        "cursor.go",
        "doc.go",
        "engine.go",
        "meta.go",
        "shard.go",
        "store.go",
    ],
    importpath = "github.com/influxdata/influxdb/tsdb",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/gogo/protobuf/proto:go_default_library",
        "//vendor/github.com/influxdata/influxdb/influxql:go_default_library",
        "//vendor/github.com/influxdata/influxdb/models:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/escape:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/limiter:go_default_library",
        "//vendor/github.com/influxdata/influxdb/toml:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb/internal:go_default_library",
        "//vendor/go.uber.org/zap:go_default_library",
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
        "//vendor/github.com/influxdata/influxdb/tsdb/engine:all-srcs",
        "//vendor/github.com/influxdata/influxdb/tsdb/internal:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
