package(default_visibility = ["//visibility:public"])

licenses(["notice"])

py_test(
    name = "verify_boilerplate",
    srcs = ["verify_boilerplate.py"],
    data = ["//:all-srcs"],
)

sh_test(
    name = "verify-pylint",
    srcs = ["verify-pylint.sh"],
    data = [
        ":pylint_bin",
        "//:all-srcs",
    ],
)

test_suite(
    name = "verify-all",
    tests = [
        "verify-pylint",
        "verify_boilerplate",
    ],
)

py_binary(
    name = "pylint_bin",
    srcs = ["pylint_bin.py"],
    deps = [
        "@pylint//:pylint",
        "@requests//:requests",  # TODO(fejta): figure out a better solution
        "@ruamel_yaml//ruamel/yaml:ruamel.yaml",
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
