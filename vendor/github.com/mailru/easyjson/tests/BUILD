load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "data.go",
        "errors.go",
        "named_type.go",
        "nested_easy.go",
        "nothing.go",
        "omitempty.go",
        "snake.go",
    ],
    importpath = "github.com/mailru/easyjson/tests",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/mailru/easyjson:go_default_library",
        "//vendor/github.com/mailru/easyjson/jwriter:go_default_library",
        "//vendor/github.com/mailru/easyjson/opt:go_default_library",
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
