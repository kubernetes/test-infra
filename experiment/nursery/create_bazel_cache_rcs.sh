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
CACHE_HOST="http://bazel-cache:8080"

make_bazel_rc () {
    echo "startup --host_jvm_args=-Dbazel.DigestFunction=sha256" >> $1
    echo "build --spawn_strategy=remote" >> $1
    echo "build --strategy=Javac=remote" >> $1
    echo "build --genrule_strategy=remote" >> $1
    echo "build --remote_rest_cache=${CACHE_HOST}" >> $1
}

# https://docs.bazel.build/versions/master/user-manual.html#bazelrc
# bazel will look for two RC files, taking the first option in each set of paths
# firstly:
# - The path specified by the --bazelrc=file startup option. If specified, this option must appear before the command name (e.g. build)
# - A file named .bazelrc in your base workspace directory
# - A file named .bazelrc in your home directory
make_bazel_rc "${HOME}/.bazelrc"
# Aside from the optional configuration file described above, Bazel also looks for a master rc file next to the binary, in the workspace at tools/bazel.rc or system-wide at /etc/bazel.bazelrc.
# These files are here to support installation-wide options or options shared between users. Reading of this file can be disabled using the --nomaster_bazelrc option.
make_bazel_rc "/etc/bazel.bazelrc"
# hopefully no repos create *both* of these ...
