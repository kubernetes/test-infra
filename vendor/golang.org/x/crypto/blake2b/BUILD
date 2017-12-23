load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "blake2b.go",
        "blake2b_generic.go",
        "blake2x.go",
        "register.go",
    ] + select({
        "@io_bazel_rules_go//go/platform:386": [
            "blake2b_ref.go",
        ],
        "@io_bazel_rules_go//go/platform:amd64": [
            "blake2bAVX2_amd64.go",
            "blake2bAVX2_amd64.s",
            "blake2b_amd64.go",
            "blake2b_amd64.s",
        ],
        "@io_bazel_rules_go//go/platform:amd64p32": [
            "blake2b_ref.go",
        ],
        "@io_bazel_rules_go//go/platform:arm": [
            "blake2b_ref.go",
        ],
        "@io_bazel_rules_go//go/platform:arm64": [
            "blake2b_ref.go",
        ],
        "@io_bazel_rules_go//go/platform:mips": [
            "blake2b_ref.go",
        ],
        "@io_bazel_rules_go//go/platform:mips64": [
            "blake2b_ref.go",
        ],
        "@io_bazel_rules_go//go/platform:mips64le": [
            "blake2b_ref.go",
        ],
        "@io_bazel_rules_go//go/platform:mipsle": [
            "blake2b_ref.go",
        ],
        "@io_bazel_rules_go//go/platform:ppc64": [
            "blake2b_ref.go",
        ],
        "@io_bazel_rules_go//go/platform:ppc64le": [
            "blake2b_ref.go",
        ],
        "@io_bazel_rules_go//go/platform:s390x": [
            "blake2b_ref.go",
        ],
        "//conditions:default": [],
    }),
    importpath = "golang.org/x/crypto/blake2b",
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
