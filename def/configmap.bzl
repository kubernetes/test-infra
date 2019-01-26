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

load("@io_bazel_rules_k8s//k8s:object.bzl", "k8s_object")

def _impl(ctx):
  args = [
    "--output=%s" % ctx.outputs.output.path,
    "--name=%s" % ctx.label.name[:-len(".generated-yaml")],
    "--namespace=%s" % ctx.attr.namespace,
  ]
  for key, value in ctx.attr.labels.items():
    args.append("--label=%s=%s" % (key, value))

  # Build the {string: label} dict
  targets = {}
  for i, t in enumerate(ctx.attr.data_strings):
    targets[t] = ctx.attr.data_labels[i]

  for name, label in ctx.attr.data.items():
    fp = targets[label].files.to_list()[0].path
    args.append(ctx.expand_location("--data=%s=%s" % (name, fp)))
  ctx.actions.run(
    inputs=ctx.files.data_labels,
    outputs=[ctx.outputs.output],
    executable=ctx.executable._writer,
    arguments=args,
    progress_message="creating %s..." % ctx.outputs.output.short_path,
  )

# See https://docs.bazel.build/versions/master/skylark/rules.html
_k8s_configmap = rule(
  implementation = _impl,
  attrs={
    # TODO(fejta): switch to string_keyed_label_dict once it exists
    "data": attr.string_dict(mandatory=True, allow_empty=False),
    "namespace": attr.string(),
    "cluster": attr.string(),
    "labels": attr.string_dict(),
    "output": attr.output(mandatory=True),
    # private attrs, the data_* are used to create a {string: label} dict
    "data_strings": attr.string_list(mandatory=True),
    "data_labels": attr.label_list(mandatory=True, allow_files=True),
    "_writer": attr.label(executable=True, cfg="host", allow_files=True,
			  default=Label("//def/configmap")),
  },
)

# A macro to create a configmap object as well as rules to manage it.
#
# Usage:
#   k8s_configmap("something", data={"foo": "//path/to/foo.json"})
#
# This is roughly equivalent to:
#    kubectl create configmap something --from-file=foo=path/to/foo.json
# Supports cluster=kubectl_context, namespace="blah", labels={"app": "fancy"}
# as well as any args k8s_object supports.
# 
# Generates a k8s_object(kind="configmap") with the generated template.
#
# See also:
#   * https://docs.bazel.build/versions/master/skylark/macros.html
#   * https://github.com/bazelbuild/rules_k8s#k8s_object
def k8s_configmap(name, data=None, namespace='', labels=None, cluster='', **kw):
  # Create the non-duplicated list of data values
  _data = data or {}
  _data_targets = {v: None for v in _data.values()}.keys()
  # Create the rule to generate the configmap
  _k8s_configmap(
    name = name + ".generated-yaml",
    data=data,
    namespace=namespace,
    labels=labels,
    output=name + "_configmap.yaml",
    data_strings=_data_targets,
    data_labels=_data_targets,
  )
  # Run k8s_object with the generated configmap
  k8s_object(
    name = name,
    kind = "configmap",
    template = name + "_configmap.yaml",
    cluster = cluster,
    namespace = namespace,
    **kw)
