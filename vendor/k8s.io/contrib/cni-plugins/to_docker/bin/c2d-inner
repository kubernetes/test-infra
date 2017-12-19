#!/bin/bash
# Copyright 2016 The Kubernetes Authors All rights reserved.
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

if [ $# -ne 2 ]; then
   echo Usage: $0 INPUT_FILE_NAME OUTPUT_FILE_NAME >& 2
   exit 1
fi
INPFN="$1"
OUTFN="$2"

# Look for the "debug" setting in the config file and turn on
# debugging if requested.
if jq .debug < "${INPFN}" | grep -i true &> /dev/null; then
    echo
    printenv | grep CNI
    set -x
fi

# From here on, any failed command is a fatal error.
set -e

case "${CNI_COMMAND}" in
    (ADD)

        # Pick the desired network name out of the config.
        thenet="$(jq -r .name < "${INPFN}")"

        # When the kubelet is configured to use a CNI plugin, the
        # infrastructure container (the one running "/pause") starts
        # out connected to the Docker network named "none".  Docker
        # does not allow a container to be connected to both "none"
        # and another network, so remove that pain.
        docker network disconnect     none "${CNI_CONTAINERID}"

        # Connect to the desired Docker network
        docker network connect "${thenet}" "${CNI_CONTAINERID}"

        # Extract the needed output info from the container
        CTR_INFO=$(docker inspect --format='{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}} {{range .NetworkSettings.Networks}}{{.Gateway}}{{end}} {{range .NetworkSettings.Networks}}{{.IPPrefixLen}}{{end}}' $CNI_CONTAINERID)
        CTR_IP=$(echo "${CTR_INFO}" | cut '-d ' -f1)
        CTR_GW=$(echo "${CTR_INFO}" | cut '-d ' -f2)
        CTR_PF=$(echo "${CTR_INFO}" | cut '-d ' -f3)

        # Produce the proper CNI output
        cat > "${OUTFN}" <<EOF
{
  "cniVersion": "0.1.0",
  "ip4": {
    "ip": "${CTR_IP}/${CTR_PF}",
    "gateway": "${CTR_GW}"
  }
}
EOF
        ;;

    (DEL)

        # Nothing needs to be done, the Docker container delete will
        # handle it all.

        cat > "${OUTFN}" <<EOF
{
  "cniVersion": "0.1.0"
}
EOF
        ;;

    (*)
        echo "Unexpected CNI_COMMAND ($CNI_COMMAND)!" >& 2
        exit 2
        ;;
esac
