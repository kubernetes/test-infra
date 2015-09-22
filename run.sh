#!/bin/sh

# Copyright 2015 The Kubernetes Authors All rights reserved.
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

/mungegithub \
  --pr-mungers=blunderbuss,lgtm-after-commit,needs-rebase,ok-to-test,path-label,ping-ci,size,submit-queue \
  --jenkins-host=http://jenkins-master:8080 \
  --jenkins-jobs=kubernetes-e2e-gce,kubernetes-e2e-gke-ci,kubernetes-build,kubernetes-e2e-gce-parallel,kubernetes-e2e-gce-autoscaling,kubernetes-e2e-gce-reboot,kubernetes-e2e-gce-scalability \
  --token-file=/etc/secret-volume/token
