load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["openutil.go"],
    importpath = "github.com/shurcooL/go/openutil",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/shurcooL/github_flavored_markdown/gfmstyle:go_default_library",
        "//vendor/github.com/shurcooL/go/gfmutil:go_default_library",
        "//vendor/github.com/shurcooL/go/open:go_default_library",
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
