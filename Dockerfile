# Copyright 2015 Google Inc. All rights reserved.
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

FROM google/debian:wheezy
MAINTAINER Brendan Burns <bburns@google.com>
RUN apt-get update
RUN apt-get install -y -qq ca-certificates
ADD path-label.txt /path-label.txt
ADD generated-files.txt /generated-files.txt
ADD blunderbuss.yml /blunderbuss.yml
# User lists for submit-queue and 'needs-ok-to-merge'
ADD committers.txt /committers.txt
ADD whitelist.txt /whitelist.txt
# Submit queue web interface
ADD www /www
ADD mungegithub /mungegithub
EXPOSE 8080
ENTRYPOINT ["/mungegithub"]
CMD ["--dry-run", "--token-file=/token"]
