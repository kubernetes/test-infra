load("@io_bazel_rules_go//go:def.bzl", "go_library", "go_test")

go_library(
    name = "go_default_library",
    srcs = [
        "bexport.go",
        "bimport.go",
        "exportdata.go",
        "gcimporter.go",
        "isAlias18.go",
        "isAlias19.go",
    ],
    importpath = "golang.org/x/tools/go/gcimporter15",
    visibility = ["//visibility:public"],
)

go_test(
    name = "go_default_test",
    srcs = ["gcimporter_test.go"],
    importpath = "golang.org/x/tools/go/gcimporter15",
    library = ":go_default_library",
)

go_test(
    name = "go_default_xtest",
    srcs = [
        "bexport19_test.go",
        "bexport_test.go",
    ],
    importpath = "golang.org/x/tools/go/gcimporter15_test",
    deps = [
        ":go_default_library",
        "//vendor/golang.org/x/tools/go/buildutil:go_default_library",
        "//vendor/golang.org/x/tools/go/loader:go_default_library",
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
