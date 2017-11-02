load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "client.go",
        "udp.go",
    ],
    importpath = "github.com/influxdata/influxdb/client/v2",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/influxdata/influxdb/models:go_default_library"],
)

go_test(
    name = "go_default_test",
    srcs = ["client_test.go"],
    importpath = "github.com/influxdata/influxdb/client/v2",
    library = ":go_default_library",
)

go_test(
    name = "go_default_xtest",
    srcs = ["example_test.go"],
    importpath = "github.com/influxdata/influxdb/client/v2_test",
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
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
