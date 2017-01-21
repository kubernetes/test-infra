package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = [
        "command_test.go",
        "comment_test.go",
        "finder_test.go",
        "interactions_test.go",
        "notification_test.go",
        "operators_test.go",
        "pinger_test.go",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = ["//vendor:github.com/google/go-github/github"],
)

go_library(
    name = "go_default_library",
    srcs = [
        "command.go",
        "comment.go",
        "finder.go",
        "interactions.go",
        "notification.go",
        "operators.go",
        "pinger.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//mungegithub/github:go_default_library",
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
