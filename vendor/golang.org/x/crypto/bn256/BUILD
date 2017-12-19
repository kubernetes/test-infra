load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "bn256.go",
        "constants.go",
        "curve.go",
        "gfp12.go",
        "gfp2.go",
        "gfp6.go",
        "optate.go",
        "twist.go",
    ],
    importpath = "golang.org/x/crypto/bn256",
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
