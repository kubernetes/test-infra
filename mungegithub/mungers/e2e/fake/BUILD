package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
)

go_library(
    name = "go_default_library",
    srcs = ["fake.go"],
    importpath = "k8s.io/test-infra/mungegithub/mungers/e2e/fake",
    deps = [
        "//mungegithub/mungers/e2e:go_default_library",
        "//mungegithub/mungers/flakesync:go_default_library",
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
