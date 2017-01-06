load("@io_bazel_rules_go//go:def.bzl", "go_prefix")

go_prefix("k8s.io/test-infra")

filegroup(
    name = "package-srcs",
    srcs = glob(["**"], exclude=["bazel-*/**", ".git/**"]),
    tags = ["automanaged"],
    visibility = ["//visibility:private"],
)

filegroup(
    name = "all-srcs",
    srcs = [
        ":package-srcs",
        "//gcsweb/cmd/gcsweb:all-srcs",
        "//gcsweb/pkg/version:all-srcs",
        "//jenkins:all-srcs",
        "//jobs:all-srcs",
        "//prow:all-srcs",
        "//scenarios:all-srcs",
        "//testgrid/config:all-srcs",
        "//testgrid/jenkins_verify:all-srcs",
        "//velodrome/fetcher:all-srcs",
        "//velodrome/sql:all-srcs",
        "//velodrome/token-counter:all-srcs",
        "//velodrome/transform:all-srcs",
        "//vendor:all-srcs",
        "//verify:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
