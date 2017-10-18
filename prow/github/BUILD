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
        "hmac_test.go",
        "links_test.go",
        "types_test.go",
    ],
    library = ":go_default_library",
)

go_library(
    name = "go_default_library",
    srcs = [
        "client.go",
        "hmac.go",
        "links.go",
        "types.go",
    ],
    deps = [
        "//vendor/github.com/shurcooL/githubql:go_default_library",
        "//vendor/golang.org/x/oauth2:go_default_library",
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
        "//prow/github/fakegithub:all-srcs",
    ],
    tags = ["automanaged"],
)
