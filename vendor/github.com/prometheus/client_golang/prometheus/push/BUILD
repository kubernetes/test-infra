load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["push.go"],
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/prometheus/client_golang/prometheus:go_default_library",
        "//vendor/github.com/prometheus/common/expfmt:go_default_library",
        "//vendor/github.com/prometheus/common/model:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = ["push_test.go"],
    library = ":go_default_library",
    deps = [
        "//vendor/github.com/prometheus/client_golang/prometheus:go_default_library",
        "//vendor/github.com/prometheus/common/expfmt:go_default_library",
    ],
)

go_test(
    name = "go_default_xtest",
    srcs = ["examples_test.go"],
    deps = [
        ":go_default_library",
        "//vendor/github.com/prometheus/client_golang/prometheus:go_default_library",
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
