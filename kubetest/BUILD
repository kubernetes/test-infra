package(default_visibility = ["//visibility:public"])

load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
    "go_test",
)

go_binary(
    name = "kubetest",
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/kubetest",
    pure = "on",
)

go_library(
    name = "go_default_library",
    srcs = [
        "anywhere.go",
        "bash.go",
        "build.go",
        "dump.go",
        "e2e.go",
        "extract_federation.go",
        "extract_k8s.go",
        "federation.go",
        "gke.go",
        "kops.go",
        "kubernetes.go",
        "main.go",
        "node.go",
        "none.go",
        "stage.go",
        "util.go",
    ],
    importpath = "k8s.io/test-infra/kubetest",
    deps = [
        "//boskos/client:go_default_library",
        "//vendor/github.com/spf13/pflag:go_default_library",
        "//vendor/golang.org/x/crypto/ssh:go_default_library",
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

go_test(
    name = "go_default_test",
    size = "small",
    srcs = [
        "anywhere_test.go",
        "dump_test.go",
        "extract_test.go",
        "kubernetes_test.go",
        "main_test.go",
        "util_test.go",
    ],
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/kubetest",
)
