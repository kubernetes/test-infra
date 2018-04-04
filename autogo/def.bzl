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

load("@io_bazel_rules_go//go:def.bzl", "go_rules_dependencies", "go_register_toolchains", "go_context")

def _impl(repo_ctx):
  print("Generating @%s//... rules for %s..." % (repo_ctx.name, repo_ctx.attr.prefix))
  # tell gazelle which package it is generating
  prefix = repo_ctx.attr.prefix
  # find the gazelle binary for the host env
  gazelle = repo_ctx.path(repo_ctx.attr._gazelle)
  # get the path to the root workspace
  root_workspace = repo_ctx.path(repo_ctx.attr._workspace).dirname

  go = repo_ctx.path(repo_ctx.attr._go)
  shadow = repo_ctx.path(repo_ctx.attr._shadow)


  # Create a shadow copy of root_workspace in the repo workspace

  # First, delete anything we created last time
  # TODO(fejta): massively unoptimized, but expedient way to ensure correctness
  _exec(repo_ctx, ["rm", "-rf", repo_ctx.path(".")])
  _exec(repo_ctx, ["touch", repo_ctx.path("BUILD.bazel")]) # required by bazel
  # Update the timestamp of the autogo binary, invalidating the autogo result cache.
  # Critically, this ensures that we reevaluate the root workspace for any changes.
  # Otherwise, if a file/package is added/deleted/moved we will not notice and not rerun gazelle.
  _exec(repo_ctx, ["touch", shadow])
  # Create a shadow copy of root workspace (clone the dir tree, symlinks in relevant files)
  _exec(repo_ctx, [go, "run", shadow, root_workspace, repo_ctx.path(".")])
  # Run gazelle to auto-create rules
  _exec(repo_ctx, [gazelle, "--repo_root", repo_ctx.path(""), "--go_prefix", repo_ctx.attr.prefix])

_autogo_generate = repository_rule(
    implementation=_impl,
    local=True,
    attrs={
        "prefix": attr.string(mandatory=True),
        "_go": attr.label(
            allow_single_file=True,
            default="@go_sdk//:bin/go"),
        "_workspace": attr.label(
            allow_single_file=True,
            default="@//:WORKSPACE"),
        "_shadow": attr.label(
            allow_single_file=True,
            default="//autogo:shadow.go"),
        "_gazelle": attr.label(
            default="@io_bazel_rules_go_repository_tools//:bin/gazelle",
            allow_single_file=True),
        },
)

def autogo_generate(name="autogo", **kw):
  """Generates automatic rules for go packages at @name (usually @autogo).

  Usage:
    autogo_generate(name="autogo", prefix="github.com/my/go/get/path")
  """
  if "go_sdk" not in native.existing_rules():
      go_rules_dependencies()
      go_register_toolchains()
  for need in ["go_sdk", "io_bazel_rules_go_repository_tools"]:
      if need not in native.existing_rules():
          fail("expected go_rules_dependencies() to load %s" % need)
  _autogo_generate(name=name, **kw)

def _exec(repo_ctx, cmd, quiet=True, **kw):
  """Run a command and fail if it returns non-zero."""
  # set quiet=False to help debugging
  ret = repo_ctx.execute(cmd, quiet=quiet, **kw)
  if ret.return_code:
    if not quiet:
      print(cmd, "returned", ret.return_code)
      print("out:", ret.stdout)
      print("err:", ret.stderr)
    fail(cmd)
  return ret
