# Copyright 2017 The Kubernetes Authors.
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

FROM ubuntu:18.04

ENV KETTLE_DB=/data/build.db TZ=America/Los_Angeles

RUN ln -snf /usr/share/zoneinfo/$TZ /etc/localtime \
    && echo $TZ > /etc/timezone \
    && apt-get update \
    && apt-get install -y \
    tzdata \
    curl \
    pv \
    time \
    sqlite3 \
    python \
    python3 \
    python3-pip \
    && rm -rf /var/lib/apt/lists/*

RUN pip3 install requests google-cloud-pubsub==0.25.0 google-cloud-bigquery==0.24.0 influxdb ruamel.yaml==0.16

RUN curl -fsSL https://bitbucket.org/squeaky/portable-pypy/downloads/pypy3.6-7.1.1-beta-linux_x86_64-portable.tar.bz2 | tar xj -C opt  && \
    ln -s /opt/pypy*/bin/pypy /usr/bin

RUN curl -o installer https://sdk.cloud.google.com && \
    bash installer --disable-prompts --install-dir=/ && \
    rm installer && \
    ln -s /google-cloud-sdk/bin/* /bin/

ADD *.py schema.json runner.sh buckets.yaml /kettle/

VOLUME ["/data"]

CMD ["/kettle/runner.sh"]
