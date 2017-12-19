load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["query.go"],
    importpath = "github.com/influxdata/influxdb/cmd/store/query",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/gogo/protobuf/proto:go_default_library",
        "//vendor/github.com/influxdata/influxdb/models:go_default_library",
        "//vendor/github.com/influxdata/influxdb/services/storage:go_default_library",
        "//vendor/github.com/influxdata/influxql:go_default_library",
        "//vendor/github.com/influxdata/yarpc:go_default_library",
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
