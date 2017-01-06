package(default_visibility = ["//visibility:public"])

licenses(["notice"])

py_test(
    name = "verify-boilerplate",
    srcs = ["verify-boilerplate.py"],
    data = ["//:all-srcs"],
)

sh_test(
    name = "verify-gofmt",
    srcs = ["verify-gofmt.sh"],
    data = [
        "//:all-srcs",
        "@io_bazel_rules_go_toolchain//:toolchain",
    ],
)

sh_test(
    name = "verify-govet",
    srcs = ["verify-govet.sh"],
    data = [
        "//:all-srcs",
        "@io_bazel_rules_go_toolchain//:toolchain",
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
