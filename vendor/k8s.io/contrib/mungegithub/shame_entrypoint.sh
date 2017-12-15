#!/bin/bash

# Copyright 2016 The Kubernetes Authors All rights reserved.
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

./mungegithub --token="${GITHUB_API_TOKEN}" --issue-reports=shame\
	      --shame-from="${SHAME_FROM}" --shame-reply-to="${SHAME_REPLY_TO}"\
	      --shame-cc="${SHAME_CC}"\
	      --shame-report-cmd="mailx -v -t -S smtp=${SMTP_SERVER} -S smtp-auth=login -S smtp-auth-user=${SMTP_USER} -S smtp-auth-password=${SMTP_PASS}" \
              --allowed-shame-domains="${ALLOWED_SHAME_DOMAINS}"
