load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "config.go",
        "downloader.go",
        "google-gcs-downloader.go",
        "parser.go",
        "perfdash.go",
        "pod_names.go",
    ],
    importpath = "k8s.io/test-infra/perfdash",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/ghodss/yaml:go_default_library",
        "//vendor/k8s.io/contrib/test-utils/utils:go_default_library",
        "//vendor/k8s.io/kubernetes/test/e2e/perftype:go_default_library",
    ],
)

go_binary(
    name = "perfdash",
    embed = [":go_default_library"],
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = ["pod_names_test.go"],
    embed = [":go_default_library"],
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
