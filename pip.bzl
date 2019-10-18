# Copyright 2017 The Bazel Authors. All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
"""Import pip requirements into Bazel."""

def _pip_import_impl(repository_ctx):
    """Core implementation of pip_import."""

    # Add an empty top-level BUILD file.
    # This is because Bazel requires BUILD files along all paths accessed
    # via //this/sort/of:path and we wouldn't be able to load our generated
    # requirements.bzl without it.
    repository_ctx.file("BUILD", "")

    # To see the output, pass: quiet=False
    result = repository_ctx.execute([
        "python3",
        repository_ctx.path(repository_ctx.attr._script),
        "--name",
        repository_ctx.attr.name,
        "--input",
        repository_ctx.path(repository_ctx.attr.requirements),
        "--output",
        repository_ctx.path("requirements.bzl"),
        "--directory",
        repository_ctx.path(""),
    ])

    if result.return_code:
        fail("pip_import failed: %s (%s)" % (result.stdout, result.stderr))

pip_import = repository_rule(
    attrs = {
        "requirements": attr.label(
            mandatory = True,
            allow_single_file = True,
        ),
        "_script": attr.label(
            executable = True,
            default = Label("@io_bazel_rules_python//tools:piptool.par"),
            cfg = "host",
        ),
    },
    implementation = _pip_import_impl,
)

"""A rule for importing <code>requirements.txt</code> dependencies into Bazel.

This rule imports a <code>requirements.txt</code> file ˙and generates a new
<code>requirements.bzl</code> file.  This is used via the <code>WORKSPACE</code>
pattern:
<pre><code>pip_import(
    name = "foo",
    requirements = ":requirements.txt",
)
load("@foo//:requirements.bzl", "pip_install")
pip_install()
</code></pre>

You can then reference imported dependencies from your <code>BUILD</code>
file with:
<pre><code>load("@foo//:requirements.bzl", "requirement")
py_library(
    name = "bar",
    ...
    deps = [
       "//my/other:dep",
       requirement("futures"),
       requirement("mock"),
    ],
)
</code></pre>

Or alternatively:
<pre><code>load("@foo//:requirements.bzl", "all_requirements")
py_binary(
    name = "baz",
    ...
    deps = [
       ":foo",
    ] + all_requirements,
)
</code></pre>

Args:
  requirements: The label of a requirements.txt file.
"""

def pip_repositories():
    """Pull in dependencies needed to use the packaging rules."""

    # At the moment this is a placeholder, in that it does not actually pull in
    # any dependencies. However, it does do some validation checking.
    #
    # As a side effect of migrating our canonical workspace name from
    # "@io_bazel_rules_python" to "@rules_python" (#203), users who still
    # imported us by the old name would get a confusing error about a
    # repository dependency cycle in their workspace. (The cycle is likely
    # related to the fact that our repo name is hardcoded into the template
    # in piptool.py.)
    #
    # To produce a more informative error message in this situation, we
    # fail-fast here if we detect that we're not being imported by the new
    # name. (I believe we have always had the requirement that we're imported
    # by the canonical name, because of the aforementioned hardcoding.)
    #
    # Users who, against best practice, do not call pip_repositories() in their
    # workspace will not benefit from this check.
    if "rules_python" not in native.existing_rules():
        message = "=" * 79 + """\n\
It appears that you are trying to import rules_python without using its
canonical name, "@rules_python". This does not work. Please change your
WORKSPACE file to import this repo with `name = "rules_python"` instead.
"""
        if "io_bazel_rules_python" in native.existing_rules():
            message += """\n\
Note that the previous name of "@io_bazel_rules_python" is no longer used.
See https://github.com/bazelbuild/rules_python/issues/203 for context.
"""
        message += "=" * 79
        fail(message)
