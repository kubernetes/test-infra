load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "file_watcher.go",
        "template.go",
    ],
    importpath = "k8s.io/contrib/ingress/controllers/nginx/nginx/template",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/fatih/structs:go_default_library",
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/gopkg.in/fsnotify.v1:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/config:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/ingress:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/sysctl:go_default_library",
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
