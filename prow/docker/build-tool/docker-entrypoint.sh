#!/bin/bash
#
# Copyright 2019 The Kubernetes Authors.
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

set -x

# enale experimental feature of docker manifest 
if [ ! -f /etc/docker/daemon.json ]; then
  mkdir -p /etc/docker
  touch /etc/docker/daemon.json
  echo "Enable experimental mode for docker "
  # fix the MTU settings for DinD daemon
  docker_mtu=8940
  jq -n --arg mtu ${docker_mtu} --arg enable true '{"mtu":$mtu|tonumber,"experimental":$enable| test("true")}' > /etc/docker/daemon.json
fi

# Start docker daemon and wait for dockerd to start
service docker start

echo "Waiting for dockerd to start..."
while :
do
  echo "Checking for running docker daemon."
  if docker ps -q > /dev/null 2>&1; then
    echo "The docker daemon is running."
    break
  fi
  sleep 1
done

function cleanup() {
  # Cleanup all docker artifacts
  docker system prune -af || true
}

trap cleanup EXIT

set +x
"$@"
EXIT_VALUE=$?
set -x

# We cleanup in the trap as well, but just in case try to clean up here as well
# shellcheck disable=SC2046
docker kill $(docker ps -q) || true
docker system prune -af || true

exit "${EXIT_VALUE}"
