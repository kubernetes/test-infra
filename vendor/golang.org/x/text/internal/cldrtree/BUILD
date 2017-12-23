load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "cldrtree.go",
        "generate.go",
        "option.go",
        "tree.go",
        "type.go",
    ],
    importpath = "golang.org/x/text/internal/cldrtree",
    visibility = ["//vendor/golang.org/x/text:__subpackages__"],
    deps = [
        "//vendor/golang.org/x/text/internal:go_default_library",
        "//vendor/golang.org/x/text/internal/gen:go_default_library",
        "//vendor/golang.org/x/text/language:go_default_library",
        "//vendor/golang.org/x/text/unicode/cldr:go_default_library",
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
        "//vendor/golang.org/x/text/internal/cldrtree/testdata/test1:all-srcs",
        "//vendor/golang.org/x/text/internal/cldrtree/testdata/test2:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
