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
    _image_tags_arm64 = "tags_arm64",
    _image_tags_ppc64le = "tags_ppc64le",
    _image_tags_s390x = "tags_s390x",
)

# prow_image is a macro for creating :app and :image targets
def prow_image(
        component,
        name,  # use "image"
        base = None,
        base_arm64 = None,
        base_ppc64le = None,
        base_s390x = None,
        stamp = True,  # stamp by default, but allow overrides
        app_name = "app",
        build_arm64 = False,
        build_ppc64le = False,
        build_s390x = False,
        symlinks_default = None,
        symlinks_arm64 = None,
        symlinks_ppc64le = None,
        symlinks_s390x = None,
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
        symlinks = symlinks_default,
        **kwargs
    )

    if build_arm64 == True:
        go_image(
            name = "%s-arm64" % app_name,
            base = base_arm64,
            embed = [":go_default_library"],
            goarch = "arm64",
            goos = "linux",
            pure = "on",
            x_defs = {"k8s.io/test-infra/prow/version.Name": component},
        )

        container_image(
            name = "%s-arm64" % name,
            base = ":%s-arm64" % app_name,
            architecture = "arm64",
            stamp = stamp,
            symlinks = symlinks_arm64,
            **kwargs
        )

    if build_ppc64le == True:
        go_image(
            name = "%s-ppc64le" % app_name,
            base = base_ppc64le,
            embed = [":go_default_library"],
            goarch = "ppc64le",
            goos = "linux",
            pure = "on",
            x_defs = {"k8s.io/test-infra/prow/version.Name": component},
        )

        container_image(
            name = "%s-ppc64le" % name,
            base = ":%s-ppc64le" % app_name,
            architecture = "ppc64le",
            stamp = stamp,
            symlinks = symlinks_ppc64le,
            **kwargs
        )

    if build_s390x == True:
        go_image(
            name = "%s-s390x" % app_name,
            base = base_s390x,
            embed = [":go_default_library"],
            goarch = "s390x",
            goos = "linux",
            pure = "on",
            x_defs = {"k8s.io/test-infra/prow/version.Name": component},
        )

        container_image(
            name = "%s-s390x" % name,
            base = ":%s-s390x" % app_name,
            architecture = "s390x",
            stamp = stamp,
            symlinks = symlinks_s390x,
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

# target_arm64 returns the arm64 image target for the command.
#
# Concretely, target("foo") returns "//prow/cmd/foo:image-arm64"
def target_arm64(cmd):
    return "//prow/cmd/%s:image-arm64" % cmd

# target_ppc64le returns the ppc64le image target for the command.
#
# Concretely, target("foo") returns "//prow/cmd/foo:image-ppc64le"
def target_ppc64le(cmd):
    return "//prow/cmd/%s:image-ppc64le" % cmd

# target_s390x returns the s390x image target for the command.
#
# Concretely, target("foo") returns "//prow/cmd/foo:image-s390x"
def target_s390x(cmd):
    return "//prow/cmd/%s:image-s390x" % cmd

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
#     "gcr.io/k8s-prow/ghproxy:20180203-deadbeef": "//ghproxy:image",
#     "gcr.io/k8s-prow/ghproxy:latest": "//ghproxy:image",
#     "gcr.io/k8s-prow/ghproxy:latest-fejta": "//ghproxy:image",
#     "gcr.io/k8s-prow-edge/hook:20180203-deadbeef": "//prow/cmd/hook:image",
#     "gcr.io/k8s-prow-edge/hook:latest": "//prow/cmd/hook:image",
#     "gcr.io/k8s-prow-edge/hook:latest-fejta": "//prow/cmd/hook:image",
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

def tags_no_edge(cmds, targets):
    # Create :YYYYmmdd-commitish :latest :latest-USER tags
    cmd_targets = {prefix(cmd): target(cmd) for cmd in cmds}
    cmd_targets.update({prefix(p): t for (p, t) in targets.items()})
    return _image_tags(cmd_targets)

# tags_arm64 returns a {image: target-arm64} map for each cmd kwarg.
def tags_arm64(cmds):
    cmd_targets = {prefix(cmd): target_arm64(cmd) for cmd in cmds}
    if EDGE_PROW_REPO:
        cmd_targets.update({edge_prefix(cmd): target_arm64(cmd) for cmd in cmds})
    return _image_tags_arm64(cmd_targets)

# tags_ppc64le returns a {image: target-ppc64le} map for each cmd kwarg.
def tags_ppc64le(cmds):
    cmd_targets = {prefix(cmd): target_ppc64le(cmd) for cmd in cmds}
    if EDGE_PROW_REPO:
        cmd_targets.update({edge_prefix(cmd): target_ppc64le(cmd) for cmd in cmds})
    return _image_tags_ppc64le(cmd_targets)

# tags_s390x returns a {image: target-s390x} map for each cmd kwarg.
def tags_s390x(cmds):
    cmd_targets = {prefix(cmd): target_s390x(cmd) for cmd in cmds}
    if EDGE_PROW_REPO:
        cmd_targets.update({edge_prefix(cmd): target_s390x(cmd) for cmd in cmds})
    return _image_tags_s390x(cmd_targets)

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

def dict_union(x, y):
    z = {}
    z.update(x)
    z.update(y)
    return z
