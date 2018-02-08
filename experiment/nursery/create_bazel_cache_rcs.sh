#!/bin/bash
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

# THIS IS TEMPORARY (!)
# Once this moves out of experiment/ we will create these files in the bazel
# images instead (!)
# TODO(bentheelder): verify that this works and move it into the images
CACHE_HOST="http://bazel-cache.default:8080"

# get the installed version of a debian package
package_to_version () {
    dpkg-query --showformat='${Version}' --show $1
}

# look up a binary with which and return the debian package it belongs to
command_to_package () {
    BINARY_PATH=$(readlink -f $(which $1))
    # `dpkg -S $package` spits out lines with the format: "package: file"
    dpkg -S $1 | grep $BINARY_PATH | cut -d':' -f1
}

# get the installed package version relating to a binary
command_to_version () {
    PACKAGE=$(command_to_package $1)
    package_to_version $PACKAGE
}

make_bazel_rc () {
    # this is the default for recent releases but we set it explicitly
    # since this is the only hash our cache supports
    echo "startup --host_jvm_args=-Dbazel.DigestFunction=sha256"
    # use remote caching for all the things
    echo "build --experimental_remote_spawn_cache"
    # point it at our http cache ...
    echo "build --remote_http_cache=${CACHE_HOST}"
    # don't fail if the cache is unavailable
    echo "build --remote_local_fallback"
    # make sure the cache considers host toolchain versions
    # NOTE: these assume nobody installs new host toolchains afterwards
    # if $CC is set bazel will use this to detect c/c++ toolchains, otherwise gcc
    # https://blog.bazel.build/2016/03/31/autoconfiguration.html
    CC="${CC:-gcc}"
    CC_VERSION=$(command_to_version $CC)
    echo "build --action_env=CACHE_GCC_VERSION=${CC_VERSION}"
    # NOTE: IIRC some rules call python internally, this can't hurt
    PYTHON_VERSION=$(command_to_version python)
    echo "build --action_env=CACHE_PYTHON_VERSION=${PYTHON_VERSION}"
}

# https://docs.bazel.build/versions/master/user-manual.html#bazelrc
# bazel will look for two RC files, taking the first option in each set of paths
# firstly:
# - The path specified by the --bazelrc=file startup option. If specified, this option must appear before the command name (e.g. build)
# - A file named .bazelrc in your base workspace directory
# - A file named .bazelrc in your home directory
make_bazel_rc >> "${HOME}/.bazelrc"
# Aside from the optional configuration file described above, Bazel also looks for a master rc file next to the binary, in the workspace at tools/bazel.rc or system-wide at /etc/bazel.bazelrc.
# These files are here to support installation-wide options or options shared between users. Reading of this file can be disabled using the --nomaster_bazelrc option.
make_bazel_rc >> "/etc/bazel.bazelrc"
# hopefully no repos create *both* of these ...
