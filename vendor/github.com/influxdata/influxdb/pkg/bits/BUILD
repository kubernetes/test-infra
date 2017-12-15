load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "bits.go",
        "bits_1.9.go",
    ],
    importpath = "github.com/influxdata/influxdb/pkg/bits",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/dgryski/go-bits:go_default_library"],
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
