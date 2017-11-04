load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["httpcache.go"],
    importpath = "github.com/gregjones/httpcache",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = ["httpcache_test.go"],
    importpath = "github.com/gregjones/httpcache",
    library = ":go_default_library",
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
        "//vendor/github.com/gregjones/httpcache/diskcache:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
