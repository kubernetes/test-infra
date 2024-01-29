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

# Update the date below (check https://hub.docker.com/_/alpine/tags) to get the
# alpine edge base image, which has a faster release cadence than alpine:latest
# (which tracks alpine stable), to pick up security patches more quickly.
FROM alpine:20231219

ARG IMAGE_ARG
ENV IMAGE=${IMAGE_ARG}

RUN apk add --no-cache ca-certificates && update-ca-certificates
