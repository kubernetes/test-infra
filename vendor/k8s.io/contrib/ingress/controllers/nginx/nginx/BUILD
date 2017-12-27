load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "command.go",
        "main.go",
        "ssl.go",
        "utils.go",
    ],
    importpath = "k8s.io/contrib/ingress/controllers/nginx/nginx",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/mitchellh/mapstructure:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/config:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/ingress:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/template:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/unversioned:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/healthz:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/flowcontrol:go_default_library",
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
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/auth:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/authreq:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/config:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/cors:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/healthcheck:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/ingress:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/ipwhitelist:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/ratelimit:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/rewrite:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/secureupstream:all-srcs",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/template:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
