# gazelle:proto legacy
# Gazelle gets confused by the config.proto file here, since it is in the same directory
# but a different package from main.go.
# Also, we can't even use the default proto rules, since we apply nonstandard tags to the
# generated code to support YAML.

package(default_visibility = ["//visibility:public"])

load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library", "go_test")
load("@io_bazel_rules_go//proto:def.bzl", "go_proto_library")

proto_library(
    name = "config_proto",
    srcs = ["config.proto"],
)

go_proto_library(
    name = "config_pb",
    importpath = "k8s.io/test-infra/testgrid/config/config_pb",
    proto = ":config_proto",
)

genrule(
    name = "gen-config",
    srcs = [
        "config.yaml",
    ],
    outs = ["testgrid-config"],
    cmd = "$(location //testgrid/config) --yaml=$< --output=$@",
    tools = [
        "//testgrid/config",
    ],
)

go_binary(
    name = "config",
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/testgrid/config",
)

go_test(
    name = "go_default_test",
    srcs = ["config_test.go"],
    data = [
        "config.yaml",
        "//jenkins/job-configs:all-srcs",
        "//mungegithub:submit-queue/deployment/kubernetes/configmap.yaml",
        "//prow:config.yaml",
    ],
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/testgrid/config",
    deps = [
        ":config_pb",
        "//prow/config:go_default_library",
        "//testgrid/config/yaml2proto:go_default_library",
        "//vendor/github.com/ghodss/yaml:go_default_library",
    ],
)

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "k8s.io/test-infra/testgrid/config",
    deps = ["//testgrid/config/yaml2proto:go_default_library"],
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
