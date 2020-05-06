# Copyright 2018 The Kubernetes Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

load("@io_bazel_rules_docker//container:image.bzl", "container_image")
load("@io_bazel_rules_docker//container:bundle.bzl", "container_bundle")
load("@io_bazel_rules_docker//contrib:push-all.bzl", "container_push")
load("@io_bazel_rules_docker//go:image.bzl", "go_image")
load("@io_bazel_rules_k8s//k8s:object.bzl", "k8s_object")
load("@io_bazel_rules_k8s//k8s:objects.bzl", "k8s_objects")
load(
    "//def:image.bzl",
    _image_tags = "tags",
)

## prow_image is a macro for creating :app and :image targets
def prow_image(
        component,
        name,  # use "image"
        base = None,
        stamp = True,  # stamp by default, but allow overrides
        app_name = "app",
        **kwargs):
    go_image(
        name = app_name,
        base = base,
        embed = [":go_default_library"],
        goarch = "amd64",
        goos = "linux",
        pure = "on",
        x_defs = {"k8s.io/test-infra/prow/version.Name": component},
    )

    container_image(
        name = name,
        base = ":" + app_name,
        stamp = stamp,
        **kwargs
    )

# prow_push creates a bundle of container images, and a target to push them.
def prow_push(
        name,
        bundle_name = "bundle",
        images = None):
    container_bundle(
        name = bundle_name,
        images = images,
    )
    container_push(
        name = name,
        bundle = ":" + bundle_name,
        format = "Docker",  # TODO(fejta): consider OCI?
    )

MULTI_KIND = None
CORE_CLUSTER = "{STABLE_PROW_CLUSTER}"  # For components like hook
BUILD_CLUSTER = "{STABLE_BUILD_CLUSTER}"  # For untrusted test code
EDGE_PROW_REPO = "{EDGE_PROW_REPO}"  # Container registry for edge images.

# prefix returns the image prefix for the command.
#
# Concretely, image("foo") returns "{STABLE_PROW_REPO}/foo"
# which usually becomes gcr.io/k8s-prow/foo
# (See hack/print-workspace-status.sh)
def prefix(cmd):
    return "{STABLE_PROW_REPO}/%s" % cmd

# edge_prefix returns the edge image prefix for the command.
#
# Concretely, image("foo") returns "{EDGE_PROW_REPO}/foo"
# which usually becomes gcr.io/k8s-prow-edge/foo
# (See hack/print-workspace-status.sh)
def edge_prefix(cmd):
    return "%s/%s" % (EDGE_PROW_REPO, cmd)

# target returns the image target for the command.
#
# Concretely, target("foo") returns "//prow/cmd/foo:image"
def target(cmd):
    return "//prow/cmd/%s:image" % cmd

# tags returns a {image: target} map for each cmd or {name: target} kwarg.
#
# In particular it will prefix the cmd image name with {STABLE_PROW_REPO} and {EDGE_PROW_REPO}
# Each image gets three tags: {DOCKER_TAG}, latest, latest-{BUILD_USER}
#
# Concretely, tags("hook", "plank", **{"ghproxy": "//ghproxy:image"}) will output the following:
#   {
#     "gcr.io/k8s-prow/hook:20180203-deadbeef": "//prow/cmd/hook:image",
#     "gcr.io/k8s-prow/hook:latest": "//prow/cmd/hook:image",
#     "gcr.io/k8s-prow/hook:latest-fejta": "//prow/cmd/hook:image",
#     "gcr.io/k8s-prow/plank:20180203-deadbeef": "//prow/cmd/plank:image",
#     "gcr.io/k8s-prow/plank:latest": "//prow/cmd/plank:image",
#     "gcr.io/k8s-prow/plank:latest-fejta": "//prow/cmd/plank:image",
#     "gcr.io/k8s-prow/ghproxy:20180203-deadbeef": "//ghproxy:image",
#     "gcr.io/k8s-prow/ghproxy:latest": "//ghproxy:image",
#     "gcr.io/k8s-prow/ghproxy:latest-fejta": "//ghproxy:image",
#     "gcr.io/k8s-prow-edge/hook:20180203-deadbeef": "//prow/cmd/hook:image",
#     "gcr.io/k8s-prow-edge/hook:latest": "//prow/cmd/hook:image",
#     "gcr.io/k8s-prow-edge/hook:latest-fejta": "//prow/cmd/hook:image",
#     "gcr.io/k8s-prow-edge/plank:20180203-deadbeef": "//prow/cmd/plank:image",
#     "gcr.io/k8s-prow-edge/plank:latest": "//prow/cmd/plank:image",
#     "gcr.io/k8s-prow-edge/plank:latest-fejta": "//prow/cmd/plank:image",
#     "gcr.io/k8s-prow-edge/ghproxy:20180203-deadbeef": "//ghproxy:image",
#     "gcr.io/k8s-prow-edge/ghproxy:latest": "//ghproxy:image",
#     "gcr.io/k8s-prow-edge/ghproxy:latest-fejta": "//ghproxy:image",
#   }
def tags(cmds, targets):
    # Create :YYYYmmdd-commitish :latest :latest-USER tags
    cmd_targets = {prefix(cmd): target(cmd) for cmd in cmds}
    cmd_targets.update({prefix(p): t for (p, t) in targets.items()})
    if EDGE_PROW_REPO:
        cmd_targets.update({edge_prefix(cmd): target(cmd) for cmd in cmds})
        cmd_targets.update({edge_prefix(p): t for (p, t) in targets.items()})
    return _image_tags(cmd_targets)

def object(name, cluster = CORE_CLUSTER, **kwargs):
    k8s_object(
        name = name,
        cluster = cluster,
        **kwargs
    )

def _basename(name):
    if "/" not in name:
        return name
    return name.rpartition("/")[-1]

# component generates k8s_object rules and returns a {kind: [targets]} map.
#
# This will generate a k8s_object rule for each specified kind.
# Use MULTI_KIND for a multi-document yaml (this returns nothing).
# Assumes files exist at <cmd>_<kind>.yaml
#
# Concretely, component("hook", "service", "deployment") will create the following:
#   object("hook_service", kind="service", template=":hook_service.yaml")
#   object("hook_deployment", kind="deployment", template=":hook_deployment.yaml")
# And return the following:
#   {
#     "hook": [":hook_service", ":hook_deployment",
#     "service": [":hook_service"],
#     "deployment": [":hook_deployment"],
#   }
def component(cmd, *kinds, **kwargs):
    targets = {}
    for k in kinds:
        if k == MULTI_KIND:
            n = cmd
        else:
            n = "%s_%s" % (cmd, k)
        kwargs["name"] = _basename(n)
        kwargs["kind"] = k
        kwargs["template"] = ":%s.yaml" % n
        object(**kwargs)
        tgt = ":%s" % n
        targets.setdefault("all", []).append(tgt)
        if k != MULTI_KIND:
            targets.setdefault(cmd, []).append(tgt)
            targets.setdefault(k, []).append(tgt)
    return targets

# release packages multiple components into a release.
#
# Generates a k8s_objects() rule for each component and kind, as well as an
# target which includes everything.
#
# Thus you can do things like:
#   bazel run //config/prow/cluster:hook.apply  # Update all hook resources
#   bazel run //config/prow/cluster:staging.apply  # Update everything on staging prow
#
# Concretely, the following:
#   release(
#     "staging",
#     component("hook", "deployment", "service"),
#     component("plank", "deployment"),
#   )
# Generates the five following rules:
#   k8s_objects(name = "hook", objects=[":hook_deployment", ":hook_service"])
#   k8s_objects(name = "plank", objects=[":plank_deployment"])
#   k8s_objects(name = "deployment", objects=[":hook_deployment", ":plank_deployment"])
#   k8s_objects(name = "service", objects=[":hook_service"])
#   k8s_objects(name = "staging", objects=[":hook_deployment", ":hook_service", ":plank_deployment"])
def release(name, *components):
    targets = {}
    objs = []
    for cs in components:
        for (n, ts) in cs.items():
            if n == "all":
                objs.extend(ts)
            else:
                targets.setdefault(n, []).extend(ts)
    for (piece, ts) in targets.items():
        k8s_objects(
            name = piece,
            objects = ts,
        )
    k8s_objects(
        name = name,
        objects = objs,
    )
