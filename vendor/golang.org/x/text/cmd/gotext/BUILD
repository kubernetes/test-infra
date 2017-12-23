load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "common.go",
        "doc.go",
        "extract.go",
        "generate.go",
        "main.go",
        "rewrite.go",
        "update.go",
    ],
    importpath = "golang.org/x/text/cmd/gotext",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/golang.org/x/text/language:go_default_library",
        "//vendor/golang.org/x/text/message/pipeline:go_default_library",
        "//vendor/golang.org/x/tools/go/buildutil:go_default_library",
        "//vendor/golang.org/x/tools/go/loader:go_default_library",
    ],
)

go_binary(
    name = "gotext",
    embed = [":go_default_library"],
    importpath = "golang.org/x/text/cmd/gotext",
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
        "//vendor/golang.org/x/text/cmd/gotext/examples/extract:all-srcs",
        "//vendor/golang.org/x/text/cmd/gotext/examples/extract_http:all-srcs",
        "//vendor/golang.org/x/text/cmd/gotext/examples/rewrite:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
