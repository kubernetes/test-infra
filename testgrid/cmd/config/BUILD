package(default_visibility = ["//visibility:public"])

load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

go_binary(
    name = "config",
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/testgrid/cmd/config",
)

go_test(
    name = "go_default_test",
    srcs = ["config_test.go"],
    data = [
        "//jenkins/job-configs:all-srcs",
        "//mungegithub:submit-queue/deployment/kubernetes/configmap.yaml",
        "//prow:config.yaml",
        "//testgrid:config-yaml",
    ],
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/testgrid/cmd/config",
    deps = [
        "//prow/config:go_default_library",
        "//testgrid:config",
        "//testgrid/yaml2proto:go_default_library",
        "//vendor/github.com/ghodss/yaml:go_default_library",
    ],
)

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "k8s.io/test-infra/testgrid/cmd/config",
    deps = ["//testgrid/yaml2proto:go_default_library"],
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
