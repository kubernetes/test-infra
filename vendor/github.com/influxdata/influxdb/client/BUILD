load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["influxdb.go"],
    importpath = "github.com/influxdata/influxdb/client",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/influxdata/influxdb/models:go_default_library"],
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "example_test.go",
        "influxdb_test.go",
    ],
    importpath = "github.com/influxdata/influxdb/client_test",
    deps = [":go_default_library"],
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
        "//vendor/github.com/influxdata/influxdb/client/v2:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
