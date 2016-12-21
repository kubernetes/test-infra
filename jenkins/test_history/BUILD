py_test(
    name = "gen_html_test",
    srcs = [
        "gen_html_test.py",
        "gen_html.py",
    ],
    tags = ["manual"],
)

py_test(
    name = "gen_json_test",
    srcs = [
        "gen_json_test.py",
        "gen_json.py",
    ],
    # Remove when these are fixed.
    # https://github.com/bazelbuild/bazel/issues/1973
    # https://github.com/bazelbuild/bazel/issues/2056
    local = True,
    tags = ["manual"],
)
