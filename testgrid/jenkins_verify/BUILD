package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
)

go_binary(
    name = "jenkins_verify",
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/testgrid/jenkins_verify",
)

go_library(
    name = "go_default_library",
    srcs = ["jenkins_validate.go"],
    importpath = "k8s.io/test-infra/testgrid/jenkins_verify",
    deps = [
        "//prow/config:go_default_library",
        "//testgrid/config/yaml2proto:go_default_library",
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
