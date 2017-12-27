load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "client.go",
        "source.go",
        "translator.go",
    ],
    importpath = "k8s.io/contrib/kubelet-to-gcm/monitor/controller",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/prometheus/common/expfmt:go_default_library",
        "//vendor/github.com/prometheus/common/model:go_default_library",
        "//vendor/google.golang.org/api/monitoring/v3:go_default_library",
        "//vendor/k8s.io/contrib/kubelet-to-gcm/monitor:go_default_library",
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
    visibility = ["//visibility:public"],
)
