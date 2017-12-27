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


FROM gcr.io/google_containers/haproxy:0.3
MAINTAINER Prashanth B <beeps@google.com>

RUN mkdir -p /etc/haproxy/errors /var/state/haproxy
RUN for ERROR_CODE in 400 403 404 408 500 502 503 504;do curl -sSL -o /etc/haproxy/errors/$ERROR_CODE.http \
	https://raw.githubusercontent.com/haproxy/haproxy-1.5/master/examples/errorfiles/$ERROR_CODE.http;done

# https://blog.phusion.nl/2015/01/20/docker-and-the-pid-1-zombie-reaping-problem
RUN curl -o /sbin/dumb-init https://github.com/Yelp/dumb-init/releases/download/v1.0.0/dumb-init_1.0.0_amd64 && \
  chmod +x /sbin/dumb-init

ENTRYPOINT ["dumb-init", "/service_loadbalancer"]

ADD haproxy.cfg /etc/haproxy/haproxy.cfg
ADD service_loadbalancer service_loadbalancer
ADD service_loadbalancer.go service_loadbalancer.go
ADD template.cfg template.cfg
ADD loadbalancer.json loadbalancer.json
ADD haproxy_reload haproxy_reload
ADD README.md README.md

RUN touch /var/run/haproxy.pid
