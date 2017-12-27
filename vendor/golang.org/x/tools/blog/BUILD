load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["blog.go"],
    importpath = "golang.org/x/tools/blog",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/golang.org/x/tools/blog/atom:go_default_library",
        "//vendor/golang.org/x/tools/present:go_default_library",
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
        "//vendor/golang.org/x/tools/blog/atom:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
