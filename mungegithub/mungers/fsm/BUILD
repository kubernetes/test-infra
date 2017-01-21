package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
)

go_library(
    name = "go_default_library",
    srcs = [
        "changes-needed.go",
        "end.go",
        "needs-review.go",
        "prereview.go",
        "states.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//mungegithub/github:go_default_library",
        "//mungegithub/mungers/matchers/comment:go_default_library",
        "//vendor:github.com/golang/glog",
        "//vendor:github.com/google/go-github/github",
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
)
