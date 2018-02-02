package(default_visibility = ["//visibility:public"])

load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")
load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")

proto_library(
    name = "config_proto",
    srcs = ["config.proto"],
)

go_proto_library(
    name = "config_go_proto",
    importpath = "k8s.io/test-infra/testgrid/config",
    proto = ":config_proto",
)

genrule(
    name = "gen-config",
    srcs = [
        "config.yaml",
    ],
    outs = ["testgrid-config"],
    cmd = "$(location //testgrid/cmd/config) --yaml=$< --output=$@",
    tools = [
        "//testgrid/cmd/config",
    ],
)

go_library(
    name = "go_default_library",
    embed = [":config_go_proto"],
    importpath = "k8s.io/test-infra/testgrid/config",
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
        "//testgrid/config/yaml2proto:all-srcs",
    ],
    tags = ["automanaged"],
)

go_test(
    name = "go_default_xtest",
    srcs = ["config_test.go"],
    data = [
        "config.yaml",
        "//jenkins/job-configs:all-srcs",
        "//mungegithub:submit-queue/deployment/kubernetes/configmap.yaml",
        "//prow:config.yaml",
    ],
    deps = [
        ":go_default_library",
        "//prow/config:go_default_library",
        "//testgrid/config/yaml2proto:go_default_library",
        "//vendor/github.com/ghodss/yaml:go_default_library",
    ],
)
