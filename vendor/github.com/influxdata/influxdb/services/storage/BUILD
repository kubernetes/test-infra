load("@io_bazel_rules_go//go:def.bzl", "go_library")

filegroup(
    name = "go_default_library_protos",
    srcs = [
        "predicate.proto",
        "storage.proto",
    ],
    visibility = ["//visibility:public"],
)

go_library(
    name = "go_default_library",
    srcs = [
        "batch_cursor.gen.go",
        "batch_cursor.go",
        "config.go",
        "eval.go",
        "expr.go",
        "predicate.go",
        "predicate.pb.go",
        "predicate_influxql.go",
        "resultset.go",
        "rpc_service.go",
        "series_cursor.go",
        "service.go",
        "storage.pb.go",
        "storage.yarpc.go",
        "store.go",
        "yarpc_server.go",
    ],
    importpath = "github.com/influxdata/influxdb/services/storage",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/gogo/protobuf/gogoproto:go_default_library",
        "//vendor/github.com/gogo/protobuf/proto:go_default_library",
        "//vendor/github.com/gogo/protobuf/types:go_default_library",
        "//vendor/github.com/influxdata/influxdb/models:go_default_library",
        "//vendor/github.com/influxdata/influxdb/monitor/diagnostics:go_default_library",
        "//vendor/github.com/influxdata/influxdb/query:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/meta:go_default_library",
        "//vendor/github.com/influxdata/influxdb/tsdb:go_default_library",
        "//vendor/github.com/influxdata/influxql:go_default_library",
        "//vendor/github.com/influxdata/yarpc:go_default_library",
        "//vendor/github.com/influxdata/yarpc/yarpcproto:go_default_library",
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
