load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "compare-logs.go",
        "compare-metrics.go",
        "compare-resources.go",
        "scraper.go",
        "util.go",
    ],
    importpath = "k8s.io/contrib/compare/src",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/daviddengcn/go-colortext:go_default_library",
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/prometheus/common/model:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/metrics:go_default_library",
        "//vendor/k8s.io/kubernetes/test/e2e:go_default_library",
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
        "//vendor/k8s.io/contrib/compare/src/main:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
