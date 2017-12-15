load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "compressed.go",
        "hll.go",
    ],
    importpath = "github.com/influxdata/influxdb/pkg/estimator/hll",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/cespare/xxhash:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/bits:go_default_library",
        "//vendor/github.com/influxdata/influxdb/pkg/estimator:go_default_library",
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
