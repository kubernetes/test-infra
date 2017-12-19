load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "estimator.go",
        "kubernetes_client.go",
        "nanny_lib.go",
    ],
    importpath = "k8s.io/contrib/addon-resizer/nanny",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/gopkg.in/inf.v0:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api/resource:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api/v1:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/clientset_generated/release_1_3:go_default_library",
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
        "//vendor/k8s.io/contrib/addon-resizer/nanny/main:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
