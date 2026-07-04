#!/usr/bin/env bash
# Copyright The Kubernetes Authors.
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

jwt_token=$(curl -sH "Metadata-Flavor: Google" "http://metadata/computeMetadata/v1/instance/service-accounts/default/identity?audience=sts.amazonaws.com&format=full&licenses=FALSE")

credentials=$(aws sts assume-role-with-web-identity --role-arn $AWS_ROLE_ARN --role-session-name $AWS_ROLE_SESSION_NAME --web-identity-token $jwt_token | jq '.Credentials' | jq '.Version=1')

echo $credentials
