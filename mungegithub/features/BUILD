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
        "aliases_test.go",
        "repo-updates_test.go",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = [
        "//mungegithub/github:go_default_library",
        "//vendor:github.com/golang/glog",
        "//vendor:github.com/google/go-github/github",
        "//vendor:k8s.io/kubernetes/pkg/util/sets",
    ],
)

go_library(
    name = "go_default_library",
    srcs = [
        "aliases.go",
        "branch-protection.go",
        "features.go",
        "google-cloud-storage.go",
        "repo-updates.go",
        "test-options.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//mungegithub/github:go_default_library",
        "//mungegithub/mungers/mungerutil:go_default_library",
        "//vendor:github.com/ghodss/yaml",
        "//vendor:github.com/golang/glog",
        "//vendor:github.com/spf13/cobra",
        "//vendor:k8s.io/kubernetes/pkg/util/sets",
        "//vendor:k8s.io/kubernetes/pkg/util/yaml",
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
)
