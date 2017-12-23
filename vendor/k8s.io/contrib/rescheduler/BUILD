load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "rescheduler.go",
        "utils.go",
    ],
    importpath = "k8s.io/contrib/rescheduler",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/spf13/pflag:go_default_library",
        "//vendor/k8s.io/contrib/cluster-autoscaler/simulator:go_default_library",
        "//vendor/k8s.io/contrib/cluster-autoscaler/utils/kubernetes:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/record:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/restclient:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/unversioned:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/fields:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/kubectl/cmd/util:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/wait:go_default_library",
        "//vendor/k8s.io/kubernetes/plugin/pkg/scheduler/schedulercache:go_default_library",
    ],
)

go_binary(
    name = "rescheduler",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/rescheduler",
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
