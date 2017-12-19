load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "bugreport.go",
        "bugreport_mock.go",
    ],
    importpath = "github.com/golang/mock/mockgen/tests/unexported_method",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/golang/mock/gomock:go_default_library"],
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
