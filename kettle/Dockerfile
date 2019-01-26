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

FROM ubuntu

RUN apt-get update && apt-get install -y \
    tzdata \
    curl \
    pv \
    time \
    sqlite3 \
    python-pip \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://bitbucket.org/squeaky/portable-pypy/downloads/pypy-5.8-1-linux_x86_64-portable.tar.bz2 | tar xj -C opt
RUN ln -s /opt/pypy*/bin/pypy /usr/bin

ADD requirements.txt /kettle/
RUN pip install -r /kettle/requirements.txt

RUN curl -o installer https://sdk.cloud.google.com && bash installer --disable-prompts --install-dir=/ && rm installer && ln -s /google-cloud-sdk/bin/* /bin/

ENV KETTLE_DB=/data/build.db
ENV TZ=America/Los_Angeles

ADD *.py schema.json runner.sh buckets.yaml /kettle/

CMD ["/kettle/runner.sh"]
VOLUME ["/data"]
