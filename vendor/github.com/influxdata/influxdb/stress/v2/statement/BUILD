load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "exec.go",
        "function.go",
        "go.go",
        "influxql.go",
        "insert.go",
        "query.go",
        "report.go",
        "response_time.go",
        "set.go",
        "statement.go",
        "template.go",
        "timestamp.go",
        "wait.go",
    ],
    importpath = "github.com/influxdata/influxdb/stress/v2/statement",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/influxdata/influxdb/client/v2:go_default_library",
        "//vendor/github.com/influxdata/influxdb/models:go_default_library",
        "//vendor/github.com/influxdata/influxdb/stress/v2/stress_client:go_default_library",
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
