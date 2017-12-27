load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "cluster_manager.go",
        "controller.go",
        "doc.go",
        "fakes.go",
        "tls.go",
        "utils.go",
    ],
    importpath = "k8s.io/contrib/ingress/controllers/gce/controller",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/google.golang.org/api/compute/v1:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/backends:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/firewalls:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/healthchecks:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/instances:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/loadbalancers:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/utils:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/apis/extensions:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/cache:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/record:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/unversioned:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/cloudprovider:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/cloudprovider/providers/gce:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/controller/framework:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/fields:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/labels:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/runtime:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/intstr:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/sets:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/wait:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/workqueue:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/watch:go_default_library",
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
