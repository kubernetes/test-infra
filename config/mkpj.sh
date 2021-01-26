#!/usr/bin/env bash
# Copyright 2020 The Kubernetes Authors.
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

# Usage: mkpj.sh --job=foo ...
#
# Arguments to this script will be passed to a dockerized mkpj
#
# Example Usage:
# config/mkpj.sh --job=post-test-infra-push-bootstrap | kubectl create -f -
# (type "master" at the Base ref prompt)
#
# NOTE: kubectl should be pointed at the prow services cluster you intend
# to create the prowjob in!
#
# You can also use bazel run //prow/cmd/mkpj instead.
# TODO: this won't be true if we move prow to it's own repo...
# https://github.com/kubernetes/test-infra/issues/11782

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
config="${root}/config/prow/config.yaml"
job_config_path="${root}/config/jobs"

docker pull gcr.io/k8s-prow/mkpj 1>&2 || true
docker run -i --rm -v "${root}:${root}:z" gcr.io/k8s-prow/mkpj "--config-path=${config}" "--job-config-path=${job_config_path}" "$@"
