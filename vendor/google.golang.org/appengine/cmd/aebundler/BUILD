load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["aebundler.go"],
    importpath = "google.golang.org/appengine/cmd/aebundler",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "aebundler",
    embed = [":go_default_library"],
    importpath = "google.golang.org/appengine/cmd/aebundler",
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
