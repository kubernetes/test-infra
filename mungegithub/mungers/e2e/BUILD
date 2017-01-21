package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = [
        "e2e_test.go",
        "resolved_test.go",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = ["//vendor:k8s.io/contrib/test-utils/utils"],
)

go_library(
    name = "go_default_library",
    srcs = [
        "e2e.go",
        "resolved.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//mungegithub/mungers/flakesync:go_default_library",
        "//vendor:github.com/golang/glog",
        "//vendor:k8s.io/contrib/test-utils/utils",
        "//vendor:k8s.io/kubernetes/pkg/util/sets",
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
        "//mungegithub/mungers/e2e/fake:all-srcs",
    ],
    tags = ["automanaged"],
)
