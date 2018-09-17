#!/bin/bash

set -e
[[ -n "${DEBUG}" ]] && set -x

bazel test //prow/...
