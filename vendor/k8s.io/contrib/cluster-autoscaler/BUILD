load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "cluster_autoscaler.go",
        "metrics.go",
        "scale_down.go",
        "scale_up.go",
        "utils.go",
    ],
    importpath = "k8s.io/contrib/cluster-autoscaler",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/prometheus/client_golang/prometheus:go_default_library",
        "//vendor/k8s.io/contrib/cluster-autoscaler/config:go_default_library",
        "//vendor/k8s.io/contrib/cluster-autoscaler/estimator:go_default_library",
        "//vendor/k8s.io/contrib/cluster-autoscaler/simulator:go_default_library",
        "//vendor/k8s.io/contrib/cluster-autoscaler/utils/gce:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api/unversioned:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/cache:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/record:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/unversioned:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/fields:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/labels:go_default_library",
        "//vendor/k8s.io/kubernetes/plugin/pkg/scheduler/schedulercache:go_default_library",
    ],
)

go_binary(
    name = "cluster-autoscaler",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/cluster-autoscaler",
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
    srcs = [
        ":package-srcs",
        "//vendor/k8s.io/contrib/cluster-autoscaler/config:all-srcs",
        "//vendor/k8s.io/contrib/cluster-autoscaler/estimator:all-srcs",
        "//vendor/k8s.io/contrib/cluster-autoscaler/simulator:all-srcs",
        "//vendor/k8s.io/contrib/cluster-autoscaler/utils/gce:all-srcs",
        "//vendor/k8s.io/contrib/cluster-autoscaler/utils/gce_url:all-srcs",
        "//vendor/k8s.io/contrib/cluster-autoscaler/utils/test:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
