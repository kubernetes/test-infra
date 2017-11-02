load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "doc.go",
        "string_flag.go",
        "template.go",
        "trace.go",
        "umask.go",
        "util.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:windows_amd64": [
            "umask_windows.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "k8s.io/kubernetes/pkg/util",
    visibility = ["//visibility:public"],
    deps = ["//vendor/github.com/golang/glog:go_default_library"],
)

go_test(
    name = "go_default_test",
    srcs = [
        "template_test.go",
        "util_test.go",
    ],
    importpath = "k8s.io/kubernetes/pkg/util",
    library = ":go_default_library",
    deps = [
        "//vendor/github.com/stretchr/testify/assert:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/diff:go_default_library",
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
        "//vendor/k8s.io/kubernetes/pkg/util/clock:all-srcs",
        "//vendor/k8s.io/kubernetes/pkg/util/flag:all-srcs",
        "//vendor/k8s.io/kubernetes/pkg/util/sets:all-srcs",
        "//vendor/k8s.io/kubernetes/pkg/util/yaml:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
