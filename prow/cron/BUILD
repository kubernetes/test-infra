load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["cron.go"],
    importpath = "k8s.io/test-infra/prow/cron",
    visibility = ["//visibility:public"],
    deps = [
        "//prow/config:go_default_library",
        "//vendor/github.com/robfig/cron:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = ["cron_test.go"],
    importpath = "k8s.io/test-infra/prow/cron",
    library = ":go_default_library",
    deps = [
        "//prow/config:go_default_library",
        "//vendor/github.com/robfig/cron:go_default_library",
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
