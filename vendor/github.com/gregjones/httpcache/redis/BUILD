load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["redis.go"],
    importpath = "github.com/gregjones/httpcache/redis",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/garyburd/redigo/redis:go_default_library",
        "//vendor/github.com/gregjones/httpcache:go_default_library",
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
