package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
)

go_binary(
    name = "migratestatus",
    library = ":go_default_library",
    tags = ["automanaged"],
)

go_library(
    name = "go_default_library",
    srcs = ["migratestatus.go"],
    tags = ["automanaged"],
    deps = [
        "//maintenance/migratestatus/migrator:go_default_library",
        "//vendor:github.com/golang/glog",
        "//vendor:github.com/google/go-github/github",
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
        "//maintenance/migratestatus/migrator:all-srcs",
    ],
    tags = ["automanaged"],
)
