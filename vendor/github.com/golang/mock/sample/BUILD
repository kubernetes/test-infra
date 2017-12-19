load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["user.go"],
    importpath = "github.com/golang/mock/sample",
    visibility = ["//visibility:public"],
    deps = [
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
    srcs = [
        ":package-srcs",
        "//vendor/github.com/golang/mock/sample/imp1:all-srcs",
        "//vendor/github.com/golang/mock/sample/imp2:all-srcs",
        "//vendor/github.com/golang/mock/sample/imp3:all-srcs",
        "//vendor/github.com/golang/mock/sample/imp4:all-srcs",
        "//vendor/github.com/golang/mock/sample/mock_user:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
