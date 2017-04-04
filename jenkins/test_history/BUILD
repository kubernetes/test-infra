py_test(
    name = "gen_html_test",
    srcs = [
        "gen_html.py",
        "gen_html_test.py",
    ],
    deps = [
        "@jinja2//:jinja2",
        "@yaml//:yaml",
    ],
)

py_test(
    name = "gen_json_test",
    srcs = [
        "gen_json.py",
        "gen_json_test.py",
    ],
    # Remove when these are fixed.
    # https://github.com/bazelbuild/bazel/issues/1973
    # https://github.com/bazelbuild/bazel/issues/2056
    local = True,
    deps = [
        "@requests//:requests",
        "@yaml//:yaml",
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
