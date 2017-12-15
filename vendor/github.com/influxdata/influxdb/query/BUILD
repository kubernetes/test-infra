load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "call_iterator.go",
        "cast.go",
        "compile.go",
        "emitter.go",
        "explain.go",
        "functions.gen.go",
        "functions.go",
        "iterator.gen.go",
        "iterator.go",
        "iterator_mapper.go",
        "linear.go",
        "monitor.go",
        "point.gen.go",
        "point.go",
        "query.go",
        "query_executor.go",
        "result.go",
        "select.go",
        "statement_rewriter.go",
        "subquery.go",
        "task_manager.go",
    ],
    importpath = "github.com/influxdata/influxdb/query",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/gogo/protobuf/proto:go_default_library",
        "//vendor/github.com/influxdata/influxdb/models:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/tracing:go_default_library",
        "//vendor/github.com/influxdata/influxdb/query/internal:go_default_library",
        "//vendor/github.com/influxdata/influxdb/query/neldermead:go_default_library",
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
    srcs = [
        ":package-srcs",
        "//vendor/github.com/influxdata/influxdb/query/internal:all-srcs",
        "//vendor/github.com/influxdata/influxdb/query/neldermead:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
