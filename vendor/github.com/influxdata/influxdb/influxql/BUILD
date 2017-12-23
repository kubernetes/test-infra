load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "ast.go",
        "call_iterator.go",
        "cast.go",
        "doc.go",
        "emitter.go",
        "functions.gen.go",
        "functions.go",
        "influxql.go",
        "iterator.gen.go",
        "iterator.go",
        "linear.go",
        "monitor.go",
        "parser.go",
        "point.gen.go",
        "point.go",
        "query_executor.go",
        "result.go",
        "sanitize.go",
        "scanner.go",
        "select.go",
        "statement_rewriter.go",
        "task_manager.go",
        "token.go",
    ],
    importpath = "github.com/influxdata/influxdb/influxql",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/gogo/protobuf/proto:go_default_library",
        "//vendor/github.com/influxdata/influxdb/influxql/internal:go_default_library",
        "//vendor/github.com/influxdata/influxdb/influxql/neldermead:go_default_library",
        "//vendor/github.com/influxdata/influxdb/models:go_default_library",
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
        "//vendor/github.com/influxdata/influxdb/influxql/internal:all-srcs",
        "//vendor/github.com/influxdata/influxdb/influxql/neldermead:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
