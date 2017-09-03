load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    visibility = ["//visibility:private"],
    deps = [
        "//mungegithub/publisher/pkg/cache:go_default_library",
        "//mungegithub/publisher/pkg/git:go_default_library",
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/renstrom/dedent:go_default_library",
        "//vendor/gopkg.in/src-d/go-git.v4:go_default_library",
        "//vendor/gopkg.in/src-d/go-git.v4/plumbing:go_default_library",
    ],
)

go_binary(
    name = "sync-tags",
    library = ":go_default_library",
    visibility = ["//visibility:public"],
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
