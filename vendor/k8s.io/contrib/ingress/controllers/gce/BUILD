load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = ["main.go"],
    importpath = "k8s.io/contrib/ingress/controllers/gce",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/spf13/pflag:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/controller:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/storage:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/unversioned:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/kubectl/cmd/util:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/labels:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/wait:go_default_library",
    ],
)

go_binary(
    name = "gce",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/ingress/controllers/gce",
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
        "//vendor/k8s.io/contrib/ingress/controllers/gce/backends:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/controller:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/firewalls:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/healthchecks:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/https_example:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/instances:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/loadbalancers:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/storage:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/utils:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
