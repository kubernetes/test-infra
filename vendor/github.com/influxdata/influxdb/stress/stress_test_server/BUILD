load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["server.go"],
    importpath = "github.com/influxdata/influxdb/stress/stress_test_server",
    visibility = ["//visibility:private"],
    deps = ["//vendor/github.com/paulbellamy/ratecounter:go_default_library"],
)

go_binary(
    name = "stress_test_server",
    embed = [":go_default_library"],
    importpath = "github.com/influxdata/influxdb/stress/stress_test_server",
    visibility = ["//visibility:public"],
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
