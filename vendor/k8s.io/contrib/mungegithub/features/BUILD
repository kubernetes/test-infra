load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "aliases.go",
        "features.go",
        "google-cloud-storage.go",
        "repo-updates.go",
        "test-options.go",
    ],
    importpath = "k8s.io/contrib/mungegithub/features",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/ghodss/yaml:go_default_library",
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/spf13/cobra:go_default_library",
        "//vendor/k8s.io/contrib/mungegithub/github:go_default_library",
        "//vendor/k8s.io/contrib/mungegithub/mungers/mungerutil:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/sets:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/yaml:go_default_library",
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
