load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["inflections.go"],
    importpath = "github.com/jinzhu/inflection",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_xtest",
    srcs = ["inflections_test.go"],
    importpath = "github.com/jinzhu/inflection_test",
    deps = ["//vendor/github.com/qor/inflection:go_default_library"],
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
