package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
    "go_test",
)

go_binary(
    name = "deck",
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/prow/cmd/deck",
    pure = "on",
    race = "off",
)

go_test(
    name = "go_default_test",
    srcs = [
        "jobs_test.go",
        "main_test.go",
    ],
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/prow/cmd/deck",
    deps = [
        "//prow/config:go_default_library",
        "//prow/kube:go_default_library",
        "//prow/pluginhelp:go_default_library",
        "//prow/tide:go_default_library",
        "//vendor/github.com/ghodss/yaml:go_default_library",
    ],
)

go_library(
    name = "go_default_library",
    srcs = [
        "jobs.go",
        "main.go",
        "pluginhelp.go",
        "tide.go",
    ],
    importpath = "k8s.io/test-infra/prow/cmd/deck",
    deps = [
        "//prow/config:go_default_library",
        "//prow/kube:go_default_library",
        "//prow/kube/labels:go_default_library",
        "//prow/pjutil:go_default_library",
        "//prow/pluginhelp:go_default_library",
        "//prow/tide:go_default_library",
        "//vendor/github.com/NYTimes/gziphandler:go_default_library",
        "//vendor/github.com/ghodss/yaml:go_default_library",
        "//vendor/github.com/sirupsen/logrus:go_default_library",
        "//vendor/k8s.io/apimachinery/pkg/util/sets:go_default_library",
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
