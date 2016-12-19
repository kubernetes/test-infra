package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
)

go_binary(
    name = "phony",
    srcs = ["main.go"],
    tags = ["automanaged"],
    deps = [
        "//prow/github:go_default_library",
        "//vendor:github.com/Sirupsen/logrus",
    ],
)
