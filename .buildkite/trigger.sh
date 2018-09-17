#!/usr/bin/env bash
set -euo pipefail
if [[ -n "${DEBUG-}" ]]; then
  set -x
fi
cd "$(dirname "$0")/../"

buildkite-agent pipeline upload "$1"
