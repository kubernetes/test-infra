load("@org_dropbox_rules_node//node:defs.bzl", "mocha_test")

py_test(
    name = "summarize_test",
    srcs = [
        "berghelroach.py",
        "summarize.py",
        "summarize_test.py",
    ],
)

py_test(
    name = "berghelroach_test",
    srcs = [
        "berghelroach.py",
        "berghelroach_test.py",
    ],
)

mocha_test(
    name = "script_test",
    srcs = ["script_test.js"],
    data = ["js-srcs"],
)

filegroup(
    name = "js-srcs",
    srcs = glob(["*.js"]),
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
