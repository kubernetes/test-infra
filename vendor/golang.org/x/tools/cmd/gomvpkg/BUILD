load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "golang.org/x/tools/cmd/gomvpkg",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/golang.org/x/tools/go/buildutil:go_default_library",
        "//vendor/golang.org/x/tools/refactor/rename:go_default_library",
    ],
)

go_binary(
    name = "gomvpkg",
    embed = [":go_default_library"],
    importpath = "golang.org/x/tools/cmd/gomvpkg",
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
