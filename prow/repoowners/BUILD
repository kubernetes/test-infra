load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = ["repoowners.go"],
    importpath = "k8s.io/test-infra/prow/repoowners",
    visibility = ["//visibility:public"],
    deps = [
        "//prow/git:go_default_library",
        "//prow/github:go_default_library",
        "//vendor/github.com/ghodss/yaml:go_default_library",
        "//vendor/github.com/sirupsen/logrus:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/sets:go_default_library",
    ],
)

go_test(
    name = "go_default_test",
    srcs = ["repoowners_test.go"],
    importpath = "k8s.io/test-infra/prow/repoowners",
    library = ":go_default_library",
    deps = [
        "//prow/git/localgit:go_default_library",
        "//prow/github/fakegithub:go_default_library",
        "//vendor/github.com/sirupsen/logrus:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/sets:go_default_library",
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
