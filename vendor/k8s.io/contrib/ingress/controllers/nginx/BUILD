load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "controller.go",
        "main.go",
        "utils.go",
    ],
    importpath = "k8s.io/contrib/ingress/controllers/nginx",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/github.com/golang/glog:go_default_library",
        "//vendor/github.com/spf13/pflag:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/auth:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/authreq:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/config:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/cors:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/healthcheck:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/ingress:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/ipwhitelist:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/ratelimit:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/rewrite:go_default_library",
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx/secureupstream:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api/errors:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/api/pod:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/apis/extensions:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/cache:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/record:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/client/unversioned:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/controller/framework:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/healthz:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/kubectl/cmd/util:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/labels:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/runtime:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/intstr:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/wait:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/util/workqueue:go_default_library",
        "//vendor/k8s.io/kubernetes/pkg/watch:go_default_library",
    ],
)

go_binary(
    name = "nginx",
    embed = [":go_default_library"],
    importpath = "k8s.io/contrib/ingress/controllers/nginx",
    visibility = ["//visibility:public"],
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
        "//vendor/k8s.io/contrib/ingress/controllers/nginx/nginx:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
