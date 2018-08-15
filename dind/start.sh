#!/bin/bash

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

wait_for_docker ()
{
  # Start docker.
  systemctl enable docker
  systemctl start docker

  # Wait for docker.
  until docker version; do sleep 1 ;done
}

start_kubelet ()
{
  # Start the kubelet.
  mkdir -p /etc/kubernetes/manifests
  mkdir -p /etc/srv/kubernetes

  # Change the kubelet to not fail with swap on.
  cat > /etc/systemd/system/kubelet.service.d/20-kubeadm.conf << EOM
[Service]
Environment="KUBELET_EXTRA_ARGS=-v4 --fail-swap-on=false"
EOM
  systemctl enable kubelet
  systemctl start kubelet
}

start_node ()
{
  mount --make-rshared /lib/modules
  wait_for_docker
  start_kubelet
  mount --make-rshared /etc/kubernetes
  mount --make-shared /run
  mount --make-shared /
  mount --make-shared /var/lib/docker
  mount --make-shared /var/lib/kubelet

  # To support arbitrary host mounts, we would need all mounts shared.
  #mount --make-rshared /

  # kube-proxy attempts to write some values into sysfs for performance. But these
  # values cannot be written outside of the original netns, even if the fs is rw.
  # This causes kube-proxy to panic if run inside dind.
  #
  # Historically, --max-conntrack or --conntrack-max-per-core could be set to 0,
  # and kube-proxy would skip the write (#25543). kube-proxy no longer respects
  # the CLI arguments if a config file is present.
  #
  # Instead, we can make sysfs ro, so that kube-proxy will forego write attempts.
  mount -o remount,ro /sys
}

start_worker ()
{
  start_node

  # Load docker images
  docker load -i /kube-proxy.tar

  # Kubeadm expects kube-proxy-amd64, but bazel names it kube-proxy
  docker tag k8s.gcr.io/kube-proxy:$(cat /docker_version) k8s.gcr.io/kube-proxy-amd64:$(cat /docker_version)

  # Start kubeadm.
  /usr/bin/kubeadm join --token=abcdef.abcdefghijklmnop --discovery-token-unsafe-skip-ca-verification=true --ignore-preflight-errors=all 172.18.0.2:6443 2>&1
}

start_master ()
{
  start_node

  # Load the docker images
  docker load -i /kube-apiserver.tar
  docker load -i /kube-controller-manager.tar
  docker load -i /kube-proxy.tar
  docker load -i /kube-scheduler.tar
  # kubeadm expects all image names to be tagged as amd64, but bazel doesn't
  # build with that suffix yet.
  docker tag k8s.gcr.io/kube-apiserver:$(cat /docker_version) k8s.gcr.io/kube-apiserver-amd64:$(cat /docker_version)
  docker tag k8s.gcr.io/kube-controller-manager:$(cat /docker_version) k8s.gcr.io/kube-controller-manager-amd64:$(cat /docker_version)
  docker tag k8s.gcr.io/kube-proxy:$(cat /docker_version) k8s.gcr.io/kube-proxy-amd64:$(cat /docker_version)
  docker tag k8s.gcr.io/kube-scheduler:$(cat /docker_version) k8s.gcr.io/kube-scheduler-amd64:$(cat /docker_version)

  cat <<EOPOLICY > /etc/kubernetes/audit-policy.yaml
apiVersion: audit.k8s.io/v1beta1
kind: Policy
omitStages:
  - "RequestReceived"
rules:
- level: RequestResponse
  resources:
  - group: "" # core
    resources: ["pods", "secrets"]
  - group: "extensions"
    resources: ["deployments"]
EOPOLICY
  chmod 0600 /etc/kubernetes/audit-policy.yaml
  cat <<EOF > /etc/kubernetes/kubeadm.conf
# Only adding items not in 'kubeadm config print-default'
apiVersion: kubeadm.k8s.io/v1alpha3
kind: InitConfiguration
kubernetesVersion: $(cat source_version | sed 's/^.//')
auditPolicy:
  path: /etc/kubernetes/audit-policy.yaml
  logDir: /etc/kubernetes/audit
featureGates:
  Auditing: true
networking:
  podSubnet: 192.168.0.0/16
bootstrapTokens:
- groups:
  - system:bootstrappers:kubeadm:default-node-token
  token: abcdef.abcdefghijklmnop
apiServerCertSANs:
$(echo $1 | sed -e 's: :\n:g' | sed 's:^:- :')
- kubernetes
# ^^^ SANs need to be in yaml list form starting from v1alpha3
EOF
  chmod 0600 /etc/kubernetes/kubeadm.conf
  # Run kubeadm init to config a master.
  /usr/bin/kubeadm -v 999 init --ignore-preflight-errors=all --config /etc/kubernetes/kubeadm.conf 2>&1
  # Unsure how to map this to the config file
  # TODO: Document this format

  # We'll want to read the kube-config from outside the container, so open read
  # permissions on admin.conf.
  chmod a+r /etc/kubernetes/admin.conf

  # Apply a pod network.
  kubectl --kubeconfig=/etc/kubernetes/admin.conf apply -f https://docs.projectcalico.org/v3.0/getting-started/kubernetes/installation/hosted/kubeadm/1.7/calico.yaml

  # Install the metrics server, and the HPA.
  kubectl --kubeconfig=/etc/kubernetes/admin.conf apply -f /addons/metrics-server/
}

start_cluster ()
{
  mount --make-rshared /
  /cluster-up -logtostderr -v=2 2>&1
}

start_host()
{
  mount --make-rshared /lib/modules
  wait_for_docker

  start_cluster
}


# Start a new process to do work.
if [[ $1 == "worker" ]] ; then
  start_worker
elif [[ $1 == "master" ]] ; then
  start_master $2
elif [[ $1 == "dind" ]] ; then
  # Don't run dindind. Just run a cluster from the current docker level.
  start_cluster
else
  # Run dindind, where the cluster lives under a single container.
  start_host
fi
