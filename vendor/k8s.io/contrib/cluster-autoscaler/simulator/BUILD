load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "cluster.go",
        "drain.go",
        "nodes.go",
        "predicates.go",
    ],
    importpath = "k8s.io/contrib/cluster-autoscaler/simulator",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api/resource:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/unversioned:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/controller:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/fields:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/kubectl/cmd:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/kubelet/types:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/runtime:go_default_library",
        "//vendor/k8s.io/kubernetes/plugin/pkg/scheduler/algorithm:go_default_library",
        "//vendor/k8s.io/kubernetes/plugin/pkg/scheduler/algorithm/predicates:go_default_library",
        "//vendor/k8s.io/kubernetes/plugin/pkg/scheduler/algorithmprovider:go_default_library",
        "//vendor/k8s.io/kubernetes/plugin/pkg/scheduler/factory:go_default_library",
        "//vendor/k8s.io/kubernetes/plugin/pkg/scheduler/schedulercache:go_default_library",
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
