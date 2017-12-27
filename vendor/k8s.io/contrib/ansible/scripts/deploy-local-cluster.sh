#!/bin/bash

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

. ./init.sh

inventory=${INVENTORY_DIR}/localhost.ini

# use localhost inventory
# run etcd playbook
# run docker playbook
# run kubernetes-master playbook
# run add-ons playbook
# run kubernetes-node playbook

# kubernetes roles takes care of token and certs generating

# skipping configure tasks as we don't want to override default configuration
# of etcd and docker.
ansible-playbook -i ${inventory} ${PLAYBOOKS_DIR}/deploy-preansible.yml "$@"
ansible-playbook -i ${inventory} ${PLAYBOOKS_DIR}/deploy-etcd.yml --skip-tags="configure" "$@"
ansible-playbook -i ${inventory} ${PLAYBOOKS_DIR}/deploy-docker.yml --skip-tags="configure" "$@"
ansible-playbook -i ${inventory} ${PLAYBOOKS_DIR}/deploy-master.yml "$@"
ansible-playbook -i ${inventory} ${PLAYBOOKS_DIR}/deploy-addons.yml "$@"
ansible-playbook -i ${inventory} ${PLAYBOOKS_DIR}/deploy-node.yml "$@"
