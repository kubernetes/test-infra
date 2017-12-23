load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["daemon.go"],
    importpath = "k8s.io/contrib/kubelet-to-gcm/monitor/main",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/spf13/pflag:go_default_library",
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/golang.org/x/oauth2/google:go_default_library",
        "//vendor/google.golang.org/api/monitoring/v3:go_default_library",
        "//vendor/k8s.io/contrib/kubelet-to-gcm/monitor:go_default_library",
        "//vendor/k8s.io/contrib/kubelet-to-gcm/monitor/config:go_default_library",
        "//vendor/k8s.io/contrib/kubelet-to-gcm/monitor/controller:go_default_library",
        "//vendor/k8s.io/contrib/kubelet-to-gcm/monitor/kubelet:go_default_library",
    ],
)

go_binary(
    name = "main",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/kubelet-to-gcm/monitor/main",
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
