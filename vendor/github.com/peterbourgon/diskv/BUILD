load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "compression.go",
        "diskv.go",
        "index.go",
    ],
    importpath = "github.com/peterbourgon/diskv",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/google/btree:go_default_library"],
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
        "//vendor/github.com/peterbourgon/diskv/examples/content-addressable-store:all-srcs",
        "//vendor/github.com/peterbourgon/diskv/examples/super-simple-store:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
