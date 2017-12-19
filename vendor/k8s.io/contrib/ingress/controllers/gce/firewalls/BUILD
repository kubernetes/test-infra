load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "fakes.go",
        "firewalls.go",
        "interfaces.go",
    ],
    importpath = "k8s.io/contrib/ingress/controllers/gce/firewalls",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/google.golang.org/api/compute/v1:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/gce/utils:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/net/sets:go_default_library",
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
