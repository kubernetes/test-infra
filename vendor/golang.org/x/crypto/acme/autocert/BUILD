load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "autocert.go",
        "cache.go",
        "listener.go",
        "renewal.go",
    ],
    importpath = "golang.org/x/crypto/acme/autocert",
    visibility = ["//visibility:public"],
    deps = ["//vendor/golang.org/x/crypto/acme:go_default_library"],
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
