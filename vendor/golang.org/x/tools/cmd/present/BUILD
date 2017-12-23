load("@io_bazel_rules_go//go:def.bzl", "go_binary", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "dir.go",
        "doc.go",
        "local.go",
        "play.go",
        "play_socket.go",
    ],
    importpath = "golang.org/x/tools/cmd/present",
    visibility = ["//visibility:private"],
    deps = [
        "//vendor/golang.org/x/tools/godoc/static:go_default_library",
        "//vendor/golang.org/x/tools/playground/socket:go_default_library",
        "//vendor/golang.org/x/tools/present:go_default_library",
    ],
)

go_binary(
    name = "present",
    embed = [":go_default_library"],
    importpath = "golang.org/x/tools/cmd/present",
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
    srcs = [":package-srcs"],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
