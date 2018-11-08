# Copyright 2017 The Kubernetes Authors.
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

def _http_archive_with_pkg_path_impl(ctx):
    """Implements the http_archive_with_pkg_path build rule."""
    ctx.execute(["mkdir", "-p", ctx.attr.pkg_path])
    ctx.download_and_extract(
        url=ctx.attr.url,
        sha256=ctx.attr.sha256,
        stripPrefix=ctx.attr.strip_prefix,
        output=ctx.attr.pkg_path)
    ctx.file(ctx.attr.pkg_path+"/BUILD.bazel", ctx.attr.build_file_content)

# http_archive_with_pkg_path extends the built-in new_http_archive with a
# pkg_path field, which can be used to specify the package installation path.
http_archive_with_pkg_path = repository_rule(
    attrs = {
        "build_file_content": attr.string(mandatory = True),
        "pkg_path": attr.string(mandatory = True),
        "sha256": attr.string(),
        "strip_prefix": attr.string(mandatory = True),
        "url": attr.string(mandatory = True),
    },
    implementation = _http_archive_with_pkg_path_impl,
)
