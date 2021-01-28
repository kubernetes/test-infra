# Copyright 2021 The Kubernetes Authors.
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

load("@npm//@bazel/rollup:index.bzl", _rollup_bundle = "rollup_bundle")
load("@npm//@bazel/terser:index.bzl", _terser_minified = "terser_minified")
load("@npm//@bazel/jasmine:index.bzl", _jasmine_node_test = "jasmine_node_test")
load("@npm//@bazel/typescript:index.bzl", _ts_library = "ts_library")

def rollup_bundle(name, format = "esm", **kw):
    _rollup_bundle(name = name, format = format, **kw)
    _terser_minified(
        name = name + ".min",
        src = name + ".js",
        sourcemap = False,
    )

def jasmine_node_test(**kw):
    _jasmine_node_test(**kw)

def ts_library(name, **kw):
    _ts_library(name = name, **kw)
