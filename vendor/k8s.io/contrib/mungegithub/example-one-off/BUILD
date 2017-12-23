load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "k8s.io/contrib/mungegithub/example-one-off",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/spf13/cobra:go_default_library",
        "//vendor/k8s.io/contrib/mungegithub/github:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/flag:go_default_library",
    ],
)

go_binary(
    name = "example-one-off",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/mungegithub/example-one-off",
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
