load("@io_bazel_rules_k8s//k8s:object.bzl", "k8s_object")

# Usage:
#   bazel run //prow/cluster:foo.apply

# TODO(fejta): consider k8s_deploy alias
# https://github.com/bazelbuild/rules_k8s#aliasing-eg-k8s_deploy

k8s_object(
    name = "branchprotector_cronjob",
    cluster = "{STABLE_PROW_CLUSTER}",  # Run make :get-cluster-credentials
    kind = "CronJob",
    template = ":branchprotector_cronjob.yaml",
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
