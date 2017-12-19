load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "dc.go",
        "time.go",
    ],
    importpath = "k8s.io/contrib/diurnal",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/unversioned:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/fields:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/labels:go_default_library",
    ],
)

go_binary(
    name = "diurnal",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/diurnal",
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
