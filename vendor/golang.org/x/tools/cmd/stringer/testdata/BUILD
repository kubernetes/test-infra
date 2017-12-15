load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "cgo.go",
        "day.go",
        "gap.go",
        "num.go",
        "number.go",
        "prime.go",
        "unum.go",
        "unum2.go",
    ],
    cgo = True,
    importpath = "golang.org/x/tools/cmd/stringer/testdata",
    visibility = ["//visibility:private"],
)

go_binary(
    name = "testdata",
    embed = [":go_default_library"],
    importpath = "golang.org/x/tools/cmd/stringer/testdata",
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
