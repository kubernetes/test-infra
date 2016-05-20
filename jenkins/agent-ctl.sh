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

# Run command with no args for usage instructions.

usage() {
  echo "Usage: $(basename "${0}") [--pr] [--real] <INSTANCE> [ACTION ...]"
  echo '  --pr: talk to the pull-request instead of e2e server'
  echo '  --real: use real sized instances'
  echo 'INSTANCE: the name of the instance to target'
  echo '  pr-: create a pr-builder-sized instance'
  echo '  light-: create an instance for light postcommit jobs (e2e)'
  echo '  heavy-: create an instance for heavy postcommit jobs (build)'
  echo '  bespoke-: create a random instance for testing'
  echo 'Actions (auto by default):'
  echo '  attach: connect the instance to the jenkins master'
  echo '  auto: detatch delete create update reboot attach'
  echo '  create: insert a new vm'
  echo '  delete: delete a vm'
  echo '  detatch: disconnect the instance from the jenkins master'
  echo '  reboot: reboot or hard reset the VM'
  echo '  update: configure prerequisite packages to run tests'
  exit 1
}


set -o nounset
set -o errexit

DOCKER_VERSION='1.9.1-0~wheezy'
GO_VERSION='go1.6.2.linux-amd64'
TIMEZONE='America/Los_Angeles'

REAL=
PR=

# Defaults
IMAGE='debian-7-backports'  # TODO(fejta): debian8
SCOPES='cloud-platform,compute-rw,storage-full'  # TODO(fejta): verify

if [[ -z "${1:-}" ]]; then
  usage
fi

while true; do
  case "${1:-}" in
    --real)
      REAL=yes
      shift
      ;;
    --pr)
      PR=yes
      shift
      ;;
    *)
      break
      ;;
  esac
done

INSTANCE="${1}"
shift
if [[ -n "${PR}" ]]; then
  echo 'Talking to PR jenkins'
  MASTER='pull-jenkins-master'
else
  MASTER='jenkins-master'
fi

if [[ "${INSTANCE}" =~ light- ]]; then
  KIND='light'
elif [[ "${INSTANCE}" =~ heavy- ]]; then
  KIND='heavy'
elif [[ "${INSTANCE}" =~ pr- ]]; then
  KIND='pr'
else
  KIND=
fi

case "${KIND}" in
  light)
    # Current experiment:
    # 20 executors, n1-highmem-16, 500G pd-standard
    # Results:
    # 5.31 cores, 70G ram, <500 write IOPs, <50MB/s write
    # Alernate experiment:
    # 2 executors, n1-highmem-2, 100G pd-standard
    # Results:
    # 0.2 cores, 1.3G ram, low IOPs (200 IOP spikes), low write (30MB/s spikes)
    DISK_SIZE='500GB'
    DISK_TYPE='pd-standard'
    MACHINE_TYPE='n1-highmem-16'
    ;;
  heavy)
    # TODO(fejta): optimize these values
    DISK_SIZE='200GB'
    DISK_TYPE='pd-standard'
    MACHINE_TYPE='n1-standard-16'
    ;;
  pr)
    # TODO(fejta): optimize these values
    DISK_SIZE='500GB'
    DISK_TYPE='pd-ssd'
    MACHINE_TYPE='n1-highmem-32'
    ;;
  *)
    ;;
esac

if [[ -z "${REAL}" ]]; then
  DISK_SIZE='200GB'
  DISK_TYPE='pd-standard'
  MACHINE_TYPE='n1-standard-1'
  read -p "Using ${MACHINE_TYPE} for testing. Continue [Y/n]: " ans
  if [[ ! "${ans}" =~ '^[yY]' ]]; then
    echo 'Add --real'
    exit 1
  fi
fi

check-kind() {
  if [[ -z "${KIND}" ]]; then
    echo "${INSTANCE} does not contain light-|heavy-|pr-"
    exit 1
  fi
}

auto-agent() {
  echo "Automatically creating ${INSTANCE}..."
  check-kind
  detatch-agent
  delete-agent
  create-agent
  update-agent
  reboot-agent
  attach-agent
}

tunnel-to-master() {
  for i in {1..10}; do
    if sudo netstat -anp | grep 8080 > /dev/null 2>&1 ; then
      sleep 1
    else
      break
    fi
  done
  sudo netstat -anp | grep 8080 && echo "8080 already used" && exit 1 \
    || echo "Tunneling to ${MASTER}..."
  gcloud compute ssh "${MASTER}" --ssh-flag='-L8080:localhost:8080' sleep 5 &
  for i in {1..10}; do
    if sudo netstat -anp | grep 8080 > /dev/null 2>&1 ; then
      break
    fi
    sleep 1
  done
  sudo netstat -anp | grep 8080 > /dev/null 2>&1 || exit 1
}


master-change() {
  tunnel-to-master
  cmd="${1}"
  ini="${HOME}/jenkins-master-creds.ini"  # /user/<user>/configure && show api token
  if [[ ! -f "${ini}" ]]; then
    echo "Missing config: ${ini}"
    exit 1
  fi
  python "$(dirname "${0}")/attach_agent.py" "${cmd}" "${INSTANCE}" "${KIND}" "${ini}" "${MASTER}"
  echo 'Waiting for tunnel to close...'
  wait
}


detatch-agent() {
  echo "Detatching ${INSTANCE}..."
  master-change delete
}


attach-agent() {
  echo "Testing gcloud works on ${INSTANCE}..."
  gcloud compute ssh "${INSTANCE}" -- gcloud compute instances list "--filter=name=${INSTANCE}"
  echo "Attaching ${INSTANCE}..."
  check-kind
  master-change create
}


delete-agent() {
  echo "Delete ${INSTANCE}..."
  if [[ -z "$(gcloud compute instances list "--filter=name=${INSTANCE}")" ]]; then
    return 0
  fi
  gcloud -q compute instances delete "${INSTANCE}"
}

create-agent() {
  echo "Create ${INSTANCE}..."
  check-kind
  gcloud compute instances create \
    "${INSTANCE}" \
    "--boot-disk-size=${DISK_SIZE}" \
    "--boot-disk-type=${DISK_TYPE}" \
    "--image=${IMAGE}" \
    "--machine-type=${MACHINE_TYPE}" \
    "--scopes=${SCOPES}" \
    "--tags=do-not-delete,jenkins"
}

update-agent() {

echo "Instantiate ${INSTANCE}..."
gcloud compute ssh "${INSTANCE}" << INSTANTIATE_DONE
set -o verbose
set -o errexit

# Install docker
which docker || curl -sSL https://get.docker.com/ | sh
id jenkins || sudo useradd jenkins -m
sudo usermod -aG docker jenkins

# Downgrade to 1.9.1
# https://github.com/kubernetes/kubernetes/issues/21451
sudo apt-get -y --force-yes install docker-engine="${DOCKER_VERSION}"
sudo docker run hello-world
sudo apt-mark hold docker-engine

# Install go (needed for hack/e2e.go)

wget "https://storage.googleapis.com/golang/${GO_VERSION}.tar.gz"
sudo tar xzvf "${GO_VERSION}.tar.gz" -C /usr/local
sudo bash -c 'GOROOT=/usr/local/go PATH=\${GOROOT}/bin:\${PATH} CGO_ENABLED=0 go install -a -installsuffix cgo std'

# install build tools
sudo apt-get -y install build-essential

# install stackdriver
curl -O https://repo.stackdriver.com/stack-install.sh
sudo bash stack-install.sh --write-gcm
curl -sSO https://dl.google.com/cloudagents/install-logging-agent.sh
sudo bash install-logging-agent.sh
rm stack-install.sh install-logging-agent.sh

# Install python
sudo apt-get -y install python-openssl python-pyasn1 python-ndg-httpsclient

# Reboot on panic
sudo touch /etc/sysctl.conf
sudo sh -c 'cat << END >> /etc/sysctl.conf
kernel.panic_on_oops = 1
kernel.panic = 10
END'
sudo sysctl -p

# Keep tmp clean
sudo apt-get -y install tmpreaper
sudo sh -c "cat << END > /etc/tmpreaper.conf
TMPREAPER_PROTECT_EXTRA=''
TMPREAPER_DIRS='/tmp/. /var/tmp/.'
TMPREAPER_TIME='3'
TMPREAPER_DELAY='256'
TMPREAPER_ADDITIONALOPTIONS=''
END"

# Configure the time zone
sudo sh -c 'echo ${TIMEZONE} > /etc/timezone'
sudo dpkg-reconfigure -f noninteractive tzdata

# Prepare jenkins workspace
sudo mkdir -p /var/lib/jenkins
sudo chown jenkins:jenkins /var/lib/jenkins
sudo su jenkins -c '
set -o verbose
set -o errexit

gcloud compute config-ssh
mkdir -p /var/lib/jenkins/gce_keys
chown jenkins:jenkins /var/lib/jenkins/gce_keys
cp ~/.ssh/google_compute_engine* /var/lib/jenkins/gce_keys/
exit 0'

# Update/upgrade
sudo apt-get -y update
sudo apt-get -y upgrade
INSTANTIATE_DONE

echo "Installing metadata cache..."
"$(dirname "${0}")/metadata-cache/metadata-cache-control.sh" remote_update "${INSTANCE}"
}

reboot-agent() {
  echo "Rebooting ${INSTANCE}..."
  gcloud compute ssh "${INSTANCE}" sudo reboot || gcloud compute instances reset "${INSTANCE}"
  sleep 120  # TODO(fejta): lame but works for now
}

if [[ -z "${1:-}" ]]; then
  auto-agent
fi

while [[ -n "${1:-}" ]]; do
  "${1}-agent"
  shift
done
