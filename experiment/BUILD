py_binary(
    name = "flakedetector",
    srcs = ["flakedetector.py"],
    deps = ["@requests//:requests"],
)

py_binary(
    name = "generate_tests",
    srcs = ["generate_tests.py"],
    data = [
        "test_config.yaml",
        "//jobs",
        "//prow:configs",
    ],
    deps = ["@ruamel_yaml//ruamel/yaml:ruamel.yaml"],
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
