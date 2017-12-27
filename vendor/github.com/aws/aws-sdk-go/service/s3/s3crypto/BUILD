load("@io_bazel_rules_go//go:def.bzl", "go_library")

go_library(
    name = "go_default_library",
    srcs = [
        "aes_cbc.go",
        "aes_cbc_content_cipher.go",
        "aes_cbc_padder.go",
        "aes_gcm.go",
        "aes_gcm_content_cipher.go",
        "cipher.go",
        "cipher_builder.go",
        "cipher_util.go",
        "decryption_client.go",
        "doc.go",
        "encryption_client.go",
        "envelope.go",
        "hash_io.go",
        "helper.go",
        "key_handler.go",
        "kms_key_handler.go",
        "mat_desc.go",
        "padder.go",
        "pkcs7_padder.go",
        "strategy.go",
    ],
    importpath = "github.com/aws/aws-sdk-go/service/s3/s3crypto",
    visibility = ["//visibility:public"],
    deps = [
        "//vendor/github.com/aws/aws-sdk-go/aws:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/aws/awserr:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/aws/client:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/aws/request:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/service/kms:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/service/kms/kmsiface:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/service/s3:go_default_library",
        "//vendor/github.com/aws/aws-sdk-go/service/s3/s3iface:go_default_library",
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
