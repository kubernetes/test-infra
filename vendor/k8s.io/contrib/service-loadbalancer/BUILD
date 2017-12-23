load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "loadbalancer_log.go",
        "service_loadbalancer.go",
    ],
    importpath = "k8s.io/contrib/service-loadbalancer",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/spf13/pflag:go_default_library",
        "//vendor/github.com/ziutek/syslog:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/cache:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/unversioned:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/controller/framework:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/fields:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/kubectl/cmd/util:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/intstr:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/wait:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/workqueue:go_default_library",
    ],
)

go_binary(
    name = "service-loadbalancer",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/service-loadbalancer",
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
