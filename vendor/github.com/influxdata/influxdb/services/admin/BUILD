load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "admin.go",
        "config.go",
        "service.go",
    ],
    importpath = "github.com/influxdata/influxdb/services/admin",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/influxdata/influxdb/services/admin/statik:go_default_library",
        "//vendor/github.com/rakyll/statik/fs:go_default_library",
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
        "//vendor/github.com/influxdata/influxdb/services/admin/statik:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
