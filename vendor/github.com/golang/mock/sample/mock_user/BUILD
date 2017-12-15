load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["mock_user.go"],
    importpath = "github.com/golang/mock/sample/mock_user",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/golang/mock/gomock:go_default_library",
        "//vendor/github.com/golang/mock/sample/imp1:go_default_library",
        "//vendor/github.com/golang/mock/sample/imp2:go_default_library",
        "//vendor/github.com/golang/mock/sample/imp3:go_default_library",
        "//vendor/github.com/golang/mock/sample/imp4:go_default_library",
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
