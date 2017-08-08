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
        "flakyjob-reporter_test.go",
        "triage-filer_test.go",
    ],
    library = ":go_default_library",
    tags = ["automanaged"],
    deps = [
        "//issue-creator/creator:go_default_library",
        "//issue-creator/testowner:go_default_library",
        "//vendor/github.com/google/go-github/github:go_default_library",
    ],
)

go_library(
    name = "go_default_library",
    srcs = [
        "flakyjob-reporter.go",
        "triage-filer.go",
    ],
    tags = ["automanaged"],
    deps = [
        "//issue-creator/creator:go_default_library",
        "//mungegithub/mungers/mungerutil:go_default_library",
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/google/go-github/github:go_default_library",
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
