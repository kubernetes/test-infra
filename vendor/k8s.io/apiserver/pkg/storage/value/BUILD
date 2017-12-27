package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
)

go_library(
    name = "go_default_library",
    srcs = ["transformer.go"],
    importpath = "k8s.io/apiserver/pkg/storage/value",
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
        "//vendor/k8s.io/apiserver/pkg/storage/value/encrypt/aes:all-srcs",
        "//vendor/k8s.io/apiserver/pkg/storage/value/encrypt/envelope:all-srcs",
        "//vendor/k8s.io/apiserver/pkg/storage/value/encrypt/identity:all-srcs",
        "//vendor/k8s.io/apiserver/pkg/storage/value/encrypt/secretbox:all-srcs",
    ],
    tags = ["automanaged"],
)
