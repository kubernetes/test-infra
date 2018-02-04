load("@io_bazel_rules_k8s//k8s:object.bzl", "k8s_object")
load("@io_bazel_rules_k8s//k8s:objects.bzl", "k8s_objects")

# Usage:
#   bazel run //prow/cluster:foo.apply

# TODO(fejta): consider k8s_deploy alias
# https://github.com/bazelbuild/rules_k8s#aliasing-eg-k8s_deploy

k8s_objects(
    name = "production",
    objects = [
        ":branchprotector_cronjob",
        ":deck_deployment",
        ":deck_service",
        ":hook_deployment",
        ":hook_service",
        ":horologium_deployment",
        ":ingress",
        ":jenkins_deployment",
        ":jenkins_service",
        ":lego_deployment",
        ":mem_limit_range",
        ":plank_deployment",
        ":prow_job",
        ":push_gateway",
        ":sinker_deployment",
        ":splice_deployment",
        ":tide_deployment",
        ":tide_service",
        ":tot_deployment",
        ":tot_service",
    ],
)

k8s_object(
    name = "branchprotector_cronjob",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "CronJob",
    template = ":branchprotector_cronjob.yaml",
)

k8s_object(
    name = "deck_deployment",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "Deployment",
    template = ":deck_deployment.yaml",
)

k8s_object(
    name = "deck_service",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "service",
    template = ":deck_service.yaml",
)

k8s_object(
    name = "hook_deployment",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "deployment",
    template = ":hook_deployment.yaml",
)

k8s_object(
    name = "hook_service",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "service",
    template = ":hook_service.yaml",
)

k8s_object(
    name = "horologium_deployment",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "deployment",
    template = ":horologium_deployment.yaml",
)

k8s_object(
    name = "ingress",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "ingress",
    template = ":ingress.yaml",
)

k8s_object(
    name = "jenkins_deployment",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "deployment",
    template = ":jenkins_deployment.yaml",
)

k8s_object(
    name = "jenkins_service",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "service",
    template = ":jenkins_service.yaml",
)

k8s_object(
    name = "lego_deployment",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "deployment",
    template = ":lego_deployment.yaml",
)

k8s_object(
    name = "mem_limit_range",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "limitrange",
    template = ":mem_limit_range.yaml",
)

k8s_object(
    name = "plank_deployment",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "deployment",
    template = ":plank_deployment.yaml",
)

k8s_object(
    name = "prow_job",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "customresourcedfinition",
    template = ":prow_job.yaml",
)

k8s_object(
    name = "push_gateway",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "deployment",
    template = ":push_gateway.yaml",
)

k8s_object(
    name = "sinker_deployment",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "deployment",
    template = ":sinker_deployment.yaml",
)

k8s_object(
    name = "splice_deployment",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "deployment",
    template = ":splice_deployment.yaml",
)

k8s_object(
    name = "starter",
    cluster = "{STABLE_PROW_CLUSTER}",
    # Multiple objects so no kind
    template = ":starter.yaml",
)

k8s_object(
    name = "tide_deployment",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "deployment",
    template = ":tide_deployment.yaml",
)

k8s_object(
    name = "tide_service",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "service",
    template = ":tide_service.yaml",
)

k8s_object(
    name = "tot_deployment",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "deployment",
    template = ":tot_deployment.yaml",
)

k8s_object(
    name = "tot_service",
    cluster = "{STABLE_PROW_CLUSTER}",
    kind = "service",
    template = ":tot_service.yaml",
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
