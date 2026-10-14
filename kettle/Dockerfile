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

FROM python:3.13-slim

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
    && rm -rf /var/lib/apt/lists/*

RUN curl https://dl.google.com/dl/cloudsdk/channels/rapid/google-cloud-sdk.tar.gz | tar xfz - -C . && \
    ./google-cloud-sdk/install.sh -q && \
    ln -s /google-cloud-sdk/bin/* /usr/local/bin/

WORKDIR /kettle

ADD *.py pyproject.toml uv.lock schema.json runner.sh buckets.yaml /kettle/

RUN --mount=from=ghcr.io/astral-sh/uv,source=/uv,target=/bin/uv \
    uv sync --locked --no-install-project --no-dev 
ENV PATH="/kettle/.venv/bin:$PATH"

VOLUME ["/data"]

CMD ["/kettle/runner.sh"]
