load("@org_pubref_rules_node//node:rules.bzl", "mocha_test")

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

#mocha_test(
#    name = "script_test",
#    main = "script_test.js",
#)

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
