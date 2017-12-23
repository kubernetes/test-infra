#!/bin/bash

# Copyright 2014 The Kubernetes Authors.
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

# used to install kubectl inside the build environment plus other tools these scripts leverage.
# uncomment for troubleshooting if required
#set -x

PKG_MANAGER=$( command -v yum || command -v apt-get ) || echo "Neither yum nor apt-get found"

#make sure sudo is installed
if [ ! -e "/usr/bin/sudo" ]; then
   ${PKG_MANAGER} install -y sudo
fi

# remove default setting of requiretty if it exists
sed -i '/Defaults requiretty/d' /etc/sudoers

#make sure wget is installed
if [ ! -e "/usr/bin/wget" ]; then
   sudo ${PKG_MANAGER} install -y wget
fi

#make sure jq is installed
if [ ! -e "/usr/bin/jq" ]; then
    sudo ${PKG_MANAGER} install -y jq
fi

#make sure envsubst is installed
if [ ! -e "/usr/bin/envsubst" ]; then
    sudo ${PKG_MANAGER} install -y gettext
fi

# make the temp directory
if [ ! -e ~/.kube ]; then
    mkdir -p ~/.kube;
fi

if [ ! -e ~/.kube/kubectl ]; then
    wget https://storage.googleapis.com/kubernetes-release/release/v1.2.2/bin/linux/amd64/kubectl -O ~/.kube/kubectl
    chmod +x ~/.kube/kubectl
fi

if (echo "${KUBECHECKSUM} `ls ~/.kube/config`" | md5sum -c -); then 
  echo kubeconfig checksum matches;
else 
  wget ${KUBEURL} -O ~/.kube/config;
fi

~/.kube/kubectl config use-context ${KUBECONTEXTQA}
~/.kube/kubectl version

## uncomment if you need to add an intermediate certificate to get push working
#sudo mkdir -p /etc/docker/certs.d/docker-registry.concur.com
#sudo wget https://s3location/digicert.crt -O /etc/docker/certs.d/docker-registry.concur.com/ca.crt