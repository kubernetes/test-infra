load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "oauth2.go",
        "token.go",
        "transport.go",
    ],
    importpath = "golang.org/x/oauth2",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/golang.org/x/net/context:go_default_library",
        "//vendor/golang.org/x/oauth2/internal:go_default_library",
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
        "//vendor/golang.org/x/oauth2/bitbucket:all-srcs",
        "//vendor/golang.org/x/oauth2/clientcredentials:all-srcs",
        "//vendor/golang.org/x/oauth2/facebook:all-srcs",
        "//vendor/golang.org/x/oauth2/fitbit:all-srcs",
        "//vendor/golang.org/x/oauth2/github:all-srcs",
        "//vendor/golang.org/x/oauth2/google:all-srcs",
        "//vendor/golang.org/x/oauth2/heroku:all-srcs",
        "//vendor/golang.org/x/oauth2/hipchat:all-srcs",
        "//vendor/golang.org/x/oauth2/internal:all-srcs",
        "//vendor/golang.org/x/oauth2/jws:all-srcs",
        "//vendor/golang.org/x/oauth2/jwt:all-srcs",
        "//vendor/golang.org/x/oauth2/linkedin:all-srcs",
        "//vendor/golang.org/x/oauth2/mediamath:all-srcs",
        "//vendor/golang.org/x/oauth2/microsoft:all-srcs",
        "//vendor/golang.org/x/oauth2/odnoklassniki:all-srcs",
        "//vendor/golang.org/x/oauth2/paypal:all-srcs",
        "//vendor/golang.org/x/oauth2/slack:all-srcs",
        "//vendor/golang.org/x/oauth2/uber:all-srcs",
        "//vendor/golang.org/x/oauth2/vk:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
