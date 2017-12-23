load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "basic.go",
        "config.go",
        "run.go",
        "template.go",
        "util.go",
    ],
    importpath = "github.com/influxdata/influxdb/stress",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/BurntSushi/toml:go_default_library",
        "//vendor/github.com/influxdata/influxdb/client/v2:go_default_library",
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
        "//vendor/github.com/influxdata/influxdb/stress/stress_test_server:all-srcs",
        "//vendor/github.com/influxdata/influxdb/stress/v2:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
