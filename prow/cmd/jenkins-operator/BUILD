package(default_visibility = ["//visibility:public"])

licenses(["notice"])

load("@io_bazel_rules_docker//go:image.bzl", "go_image")
load(
    "@io_bazel_rules_go//go:def.bzl",
    "go_binary",
    "go_library",
)

go_image(
    name = "image",
    base = "@alpine-base//image",
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/prow/cmd/jenkins-operator",
    pure = "on",
    race = "off",
)

go_library(
    name = "go_default_library",
    srcs = [
        "logs.go",
        "main.go",
    ],
    importpath = "k8s.io/test-infra/prow/cmd/jenkins-operator",
    deps = [
        "//prow/config:go_default_library",
        "//prow/github:go_default_library",
        "//prow/jenkins:go_default_library",
        "//prow/kube:go_default_library",
        "//prow/kube/labels:go_default_library",
        "//prow/metrics:go_default_library",
        "//vendor/github.com/NYTimes/gziphandler:go_default_library",
        "//vendor/github.com/prometheus/client_golang/prometheus/promhttp:go_default_library",
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
    srcs = [":package-srcs"],
    tags = ["automanaged"],
)

go_binary(
    name = "jenkins-operator",
    embed = [":go_default_library"],
    importpath = "k8s.io/test-infra/prow/cmd/jenkins-operator",
    pure = "on",
    race = "off",
)
