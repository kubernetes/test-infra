package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_library",
    "go_test",
)

go_test(
    name = "go_default_test",
    srcs = [
        "client_test.go",
        "prowjob_test.go",
    ],
    importpath = "k8s.io/test-infra/prow/kube",
    library = ":go_default_library",
)

go_library(
    name = "go_default_library",
    srcs = [
        "client.go",
        "prowjob.go",
        "types.go",
    ],
    importpath = "k8s.io/test-infra/prow/kube",
    deps = [
        "//vendor/github.com/ghodss/yaml:go_default_library",
        "//vendor/github.com/sirupsen/logrus:go_default_library",
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
        "//prow/kube/labels:all-srcs",
    ],
    tags = ["automanaged"],
)
