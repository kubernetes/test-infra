#!/usr/bin/env python3

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

import os
import sys

import pylint

if __name__ == '__main__':
    # Otherwise bazel's symlinks confuse pylint/astroid
    EXTRAS = set()
    for path in sys.path:
        if not os.path.isdir(path):
            continue
        for something in os.listdir(path):
            # bazel stopped symlinking dirs, so this now depends on files like
            # bazel-out/k8-fastbuild/bin/hack/verify-pylint.runfiles/pypi__sh_1_12_14/ pylint: disable=line-too-long
            # containing a sh.py symlink to external/pypi__sh_1_12_14/sh.py
            # imports that do not have files in the root dir will not have a
            # different real name.
            full = os.path.join(path, something)
            real = os.path.realpath(full)
            # If we use pip_import() then there will be
            # a WHEEL file symlink we can use to find the real path.
            # TODO(fejta): https://github.com/kubernetes/test-infra/issues/13162
            # (ruamel has C code, which hasn't yet worked with pip_import())
            if real == full: # bazel stopped symlinking dirs
                wheel = os.path.join(full, 'WHEEL')
                if not os.path.isfile(wheel):
                    continue
                real = os.path.dirname(os.path.realpath(wheel))
            if real == full:
                continue
            EXTRAS.add(os.path.dirname(real))
            break
    # also do one level up so foo.bar imports work :shrug:
    EXTRAS = set(os.path.dirname(e) for e in EXTRAS).union(EXTRAS)
    # append these to the path
    sys.path.extend(EXTRAS)

    # Otherwise this is the entirety of bin/pylint
    pylint.run_pylint()
