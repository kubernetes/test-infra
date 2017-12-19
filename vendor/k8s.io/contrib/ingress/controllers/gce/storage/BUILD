load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "configmaps.go",
        "doc.go",
        "pools.go",
    ],
    importpath = "k8s.io/contrib/ingress/controllers/gce/storage",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api/errors:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/cache:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/unversioned:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/wait:go_default_library",
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
