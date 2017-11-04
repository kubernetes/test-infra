load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "alert.go",
        "fingerprinting.go",
        "labels.go",
        "labelset.go",
        "metric.go",
        "model.go",
        "signature.go",
        "silence.go",
        "time.go",
        "value.go",
    ],
    importpath = "github.com/prometheus/common/model",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = [
        "labels_test.go",
        "metric_test.go",
        "signature_test.go",
        "time_test.go",
        "value_test.go",
    ],
    importpath = "github.com/prometheus/common/model",
    library = ":go_default_library",
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
