load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "controller.go",
        "keepalived.go",
        "main.go",
        "utils.go",
    ],
    importpath = "k8s.io/contrib/keepalived-vip",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/spf13/pflag:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api/errors:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/cache:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/unversioned:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/controller/framework:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/fields:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/kubectl/cmd/util:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/labels:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/dbus:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/exec:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/flowcontrol:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/intstr:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/iptables:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/node:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/sysctl:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/wait:go_default_library",
    ],
)

go_binary(
    name = "keepalived-vip",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/keepalived-vip",
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
