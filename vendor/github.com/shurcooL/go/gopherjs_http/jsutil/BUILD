load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["jsutil.go"],
    importpath = "github.com/shurcooL/go/gopherjs_http/jsutil",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/gopherjs/gopherjs/js:go_default_library",
        "//vendor/honnef.co/go/js/dom:go_default_library",
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
