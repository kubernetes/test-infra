load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "convert.go",
        "doc.go",
        "poll.go",
    ],
    importpath = "k8s.io/contrib/kubelet-to-gcm/monitor",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/google.golang.org/api/monitoring/v3:go_default_library",
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
    srcs = [
        ":package-srcs",
        "//vendor/k8s.io/contrib/kubelet-to-gcm/monitor/config:all-srcs",
        "//vendor/k8s.io/contrib/kubelet-to-gcm/monitor/controller:all-srcs",
        "//vendor/k8s.io/contrib/kubelet-to-gcm/monitor/kubelet:all-srcs",
        "//vendor/k8s.io/contrib/kubelet-to-gcm/monitor/main:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
