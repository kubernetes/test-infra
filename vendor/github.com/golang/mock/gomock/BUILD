load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "call.go",
        "callset.go",
        "controller.go",
        "matchers.go",
    ],
    importpath = "github.com/golang/mock/gomock",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "controller_test.go",
        "matchers_test.go",
    ],
    importpath = "github.com/golang/mock/gomock_test",
    deps = [
        ":go_default_library",
        "//vendor/github.com/golang/mock/gomock/mock_matcher:go_default_library",
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
