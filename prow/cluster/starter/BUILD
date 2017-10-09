load("@k8s_configmap//:defaults.bzl", "k8s_configmap")

[k8s_configmap(
    name = name + "-configmap",
    template = ":" + name + ".configmap.yaml",
) for name in [
    "config",
    "plugins",
]]

load("@k8s_deployment//:defaults.bzl", "k8s_deployment")

# The image names within each of the templates.
images = {
    "deck": "gcr.io/k8s-prow/deck:0.52",
    "hook": "gcr.io/k8s-prow/hook:0.172",
    "horologium": "gcr.io/k8s-prow/horologium:0.8",
    "plank": "gcr.io/k8s-prow/plank:0.49",
    "sinker": "gcr.io/k8s-prow/sinker:0.21",
}

[k8s_deployment(
    name = name + "-deployment",
    images = {
        images[name]: "//prow/cmd/" + name + ":image",
    },
    template = ":" + name + ".deployment.yaml",
) for name in [
    "deck",
    "hook",
    "horologium",
    "plank",
    "sinker",
]]

load("@k8s_service//:defaults.bzl", "k8s_service")

[k8s_service(
    name = name + "-service",
    template = ":" + name + ".service.yaml",
) for name in [
    "deck",
    "hook",
]]

load("@k8s_secret//:defaults.bzl", "k8s_secret")

# If you want to create secrets along with everything else, put
# something like this in WORKSPACE, with foo.secret.yaml objects.
# local_repository(
#     name = "prow_secrets",
#     path = "../secrets",
# )
# Then you can uncomment this, and the dependency below.
# [k8s_secret(
#     name = name + "-secret",
#     template = "@prow_secrets//:" + name + ".secret.yaml",
# ) for name in ["hmac", "oauth"]]
# # The secrets for Prow, make changes and run:
# #   bazel run :secrets.apply
# # ... to redeploy changes.
# k8s_objects(
#     name = "secrets",
#     objects = [
#         ":hmac-secret",
#         ":oauth-secret",
#     ],
# )

load("@k8s_tpr//:defaults.bzl", "k8s_tpr")

k8s_tpr(
    name = "prowjob-tpr",
    template = ":prowjob.tpr.yaml",
)

load("@k8s_ingress//:defaults.bzl", "k8s_ingress")

k8s_ingress(
    name = "prow-ingress",
    template = ":prow.ingress.yaml",
)

load("@io_bazel_rules_k8s//k8s:objects.bzl", "k8s_objects")

# The configuration files for Prow, make changes to these files and run:
#   bazel run :configmaps.apply
# ... to update them on the cluster.
k8s_objects(
    name = "configmaps",
    objects = [
        ":config-configmap",
        ":plugins-configmap",
    ],
)

# The deployments for Prow, make changes and run:
#   bazel run :deployments.apply
# ... to redeploy.
k8s_objects(
    name = "deployments",
    objects = [
        ":deck-deployment",
        ":hook-deployment",
        ":horologium-deployment",
        ":plank-deployment",
        ":sinker-deployment",
    ],
)

# A new cluster can be stood up using:
#   bazel run :everything.apply
# Then (most) individual components can be replaced using:
#   bazel run :foo-bar.apply
k8s_objects(
    name = "everything",
    objects = [
        # Uncomment this (along with its definition above)
        # to create secrets along with all else.
        # ":secrets",
        ":configmaps",
        ":deployments",
        ":deck-service",
        ":hook-service",
        ":prowjob-tpr",
        ":prow-ingress",
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
