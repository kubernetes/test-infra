load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["exechealthz.go"],
    importpath = "k8s.io/contrib/exec-healthz/cmd/exechealthz",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/k8s.io/contrib/exec-healthz/pkg/version:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/clock:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/exec:go_default_library",
    ],
)

go_binary(
    name = "exechealthz",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/exec-healthz/cmd/exechealthz",
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
