# The aws-janitor is not properly vendored in this repo
# gazelle:exclude aws-janitor

py_test(
    name = "bootstrap_test",
    srcs = [
        "bootstrap.py",
        "bootstrap_test.py",
    ],
    data = [
        "//jenkins/job-configs:configs",
        "//jenkins/job-configs/kubernetes-jenkins:configs",
        "//jenkins/job-configs/kubernetes-jenkins-pull:configs",
        "//jobs",
        "//prow:config.yaml",
    ],
    deps = ["@yaml//:yaml"],
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
        "//jenkins/job-configs:all-srcs",
    ],
    tags = ["automanaged"],
    visibility = ["//visibility:public"],
)
