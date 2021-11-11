# Copyright 2021 The Kubernetes Authors.
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

FROM python:3.8-slim-buster

WORKDIR /app

COPY metrics/get_jq.sh /app/get_jq.sh
COPY requirements3.txt /app/requirements3.txt

RUN /bin/bash -c "apt-get update && apt-get install -y make wget"
RUN /bin/bash -c "pip3 install -r requirements3.txt"
RUN /bin/bash -c "pip3 install certifi chardet google-api-core google-cloud-bigquery google-cloud-pubsub idna python-dateutil pytz requests six urllib3"
RUN /bin/bash -c "chmod a+x get_jq.sh && ./get_jq.sh && mv jq-1.5 .."

CMD ["make", "pytests"]
