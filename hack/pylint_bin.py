#!/usr/bin/env python2

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
            full = os.path.join(path, something)
            real = os.path.realpath(full)
            if real != full:
                EXTRAS.add(os.path.dirname(real))
                break
    # also do one level up so foo.bar imports work :shrug:
    EXTRAS = set(os.path.dirname(e) for e in EXTRAS).union(EXTRAS)
    # append these to the path
    sys.path.extend(EXTRAS)

    # Otherwise this is the entirety of bin/pylint
    pylint.run_pylint()
