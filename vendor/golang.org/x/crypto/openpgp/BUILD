load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "canonical_text.go",
        "keys.go",
        "read.go",
        "write.go",
    ],
    importpath = "golang.org/x/crypto/openpgp",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/golang.org/x/crypto/openpgp/armor:go_default_library",
        "//vendor/golang.org/x/crypto/openpgp/errors:go_default_library",
        "//vendor/golang.org/x/crypto/openpgp/packet:go_default_library",
        "//vendor/golang.org/x/crypto/openpgp/s2k:go_default_library",
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
        "//vendor/golang.org/x/crypto/openpgp/armor:all-srcs",
        "//vendor/golang.org/x/crypto/openpgp/clearsign:all-srcs",
        "//vendor/golang.org/x/crypto/openpgp/elgamal:all-srcs",
        "//vendor/golang.org/x/crypto/openpgp/errors:all-srcs",
        "//vendor/golang.org/x/crypto/openpgp/packet:all-srcs",
        "//vendor/golang.org/x/crypto/openpgp/s2k:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
