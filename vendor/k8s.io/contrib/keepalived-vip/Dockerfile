# Copyright 2015 The Kubernetes Authors. All rights reserved.
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

FROM gcr.io/google_containers/ubuntu-slim:0.4

RUN apt-get update && apt-get install -y --no-install-recommends \
  libssl1.0.0 \
  libnl-3-200 \
  libnl-route-3-200 \
  libnl-genl-3-200 \
  iptables \
  libnfnetlink0 \
  libiptcdata0 \
  libipset3 \
  libipset-dev \
  libsnmp30 \
  kmod \
  ca-certificates \
  iproute2 \
  ipvsadm \
  bash && \
  apt-get clean && \
  rm -rf /var/lib/apt/lists/*

ADD keepalived.tar.gz /

RUN mkdir -p /etc/keepalived && \
  ln -s /keepalived/sbin/keepalived /usr/sbin && \
  ln -s /keepalived/bin/genhash /usr/sbin

COPY kube-keepalived-vip /
COPY keepalived.tmpl /
COPY keepalived.conf /etc/keepalived

ENTRYPOINT ["/kube-keepalived-vip"]
