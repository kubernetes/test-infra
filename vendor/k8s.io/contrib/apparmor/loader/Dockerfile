# Copyright 2016 The Kubernetes Authors.
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

FROM alpine:3.4
MAINTAINER stclair@google.com

ENV GLIBC_VERSION "2.23-r3"

RUN apk --no-cache add \
    binutils-libs \
    ca-certificates \
    libgcc \
    wget

# Install the 3p glibc apk.
RUN wget -q -O /etc/apk/keys/sgerrand.rsa.pub https://raw.githubusercontent.com/sgerrand/alpine-pkg-glibc/master/sgerrand.rsa.pub && \
    wget https://github.com/sgerrand/alpine-pkg-glibc/releases/download/${GLIBC_VERSION}/glibc-${GLIBC_VERSION}.apk && \
    wget https://github.com/andyshinn/alpine-pkg-glibc/releases/download/${GLIBC_VERSION}/glibc-bin-${GLIBC_VERSION}.apk && \
    apk add glibc-${GLIBC_VERSION}.apk glibc-bin-${GLIBC_VERSION}.apk && \
    rm -rf glibc{,-bin}-${GLIBC_VERSION}.apk

# The apparmor package includes a lot of unnecessary dependencies, so we manually copy the necessary
# files.
ADD libapparmor.so.1 /usr/lib/libapparmor.so.1
ADD apparmor_parser /sbin/apparmor_parser

# Reload the libraries.
RUN /usr/glibc-compat/sbin/ldconfig /lib /usr/glibc-compat/lib

# Main binary.
ADD loader /usr/bin/loader

ENTRYPOINT ["/usr/bin/loader", "-logtostderr", "-v=2"]

# Default directory to watch.
CMD ["/profiles"]
