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
  echo "Usage: $(basename "${0}") [--FLAGS] <INSTANCE> [ACTION ...]"
  echo 'Flags:'
  echo '  --base-image: create instance with base image instead of family'
  echo '  --pr: talk to the pull-request instead of e2e server'
  echo '  --real: use real sized instances'
  echo 'INSTANCE: the name of the instance to target'
  echo '  pr-: create a pr-builder-sized instance'
  echo '  light-: create an instance for light postcommit jobs (e2e)'
  echo '  heavy-: create an instance for heavy postcommit jobs (build)'
  echo 'Actions (auto by default):'
  echo '  attach: connect the instance to the jenkins master'
  echo '  auto: detatch delete create attach'
  echo '  auto-image: delete create update copy-keys reboot update-image delete'
  echo '  copy-keys: copy ssh keys from master to agent'
  echo '  create: insert a new vm'
  echo '  create-image: create a new agent image'
  echo '  delete: delete a vm'
  echo '  detatch: disconnect the instance from the jenkins master'
  echo '  reboot: reboot or hard reset the VM'
  echo '  update: configure prerequisite packages to run tests'
  echo '  update-image: update the image-family used to create new disks'
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
BASE_IMAGE='debian-7-backports'  # TODO(fejta): debian8
IMAGE='jenkins-agent'
IMAGE_FLAG="--image-family=${IMAGE}"
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
    --base-image)
      IMAGE_FLAG="--image=${BASE_IMAGE}"
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
    # 14 agents
    # 10 executors, n1-highmem-8, 250G pd-standard
    # Results:
    # (1.0 cores, 20G active ram)
    # 1.46 cores, 50G ram, <250 write IOPs, <25MB/s write
    # Results:
    # 0.2 cores, 1.3G ram, low IOPs (200 IOP spikes), low write (30MB/s spikes)
    DISK_SIZE='250GB'
    DISK_TYPE='pd-standard'
    MACHINE_TYPE='n1-highmem-8'
    ;;
  heavy)
    # Current experiment:
    # 8 agents
    # 1 executor, n1-highmem-8, 150G pd-standard
    # Results:
    # 14-32 cores, 12G ram, 150 write IOPs, <20MB/s write
    DISK_SIZE='150GB'
    DISK_TYPE='pd-standard'
    MACHINE_TYPE='n1-standard-8'
    ;;
  pr)
    # Current experiment:
    # 5 agents
    # 6 executors, n1-highmem-32, 500G ssd
    # Results:
    # 80/60/40 cores, 80/60G ram, <250 IOPs, <50MB/s
    # New experiment:
    # 3 executors, n1-standard-16, 500G pd
    # Results:
    # 40/25/20 cores, 52/40G ram, <250 IOPs, <32MB/s
    # Newer experiment:
    # 1 executor, n1-standard-8, 200G pd
    # Results:
    DISK_SIZE='200GB'
    DISK_TYPE='pd-standard'
    MACHINE_TYPE='n1-standard-8'
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
  copy-keys-agent
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
  echo "Checking presence of ssh keys on ${INSTANCE}..."
  gcloud compute ssh "${INSTANCE}" -- "[[ -f /var/lib/jenkins/gce_keys/google_compute_engine ]]"
  echo "Attaching ${INSTANCE}..."
  check-kind
  master-change create
}


delete-agent() {
  echo "Delete ${INSTANCE}..."
  if [[ -z "$(gcloud compute instances list --filter="name=${INSTANCE}")" ]]; then
    return 0
  fi
  gcloud -q compute instances delete "${INSTANCE}"
}

auto-image-agent() {
  delete-agent
  create-agent
  update-agent
  copy-keys-agent
  reboot-agent
  update-image-agent
  delete-agent
}

update-image-agent() {
  family="${IMAGE}"
  image="${family}-$(date +%Y%m%d-%H%M)"
  echo "Create ${image} for ${family} from ${INSTANCE}..."
  echo "  Create snapshot of ${INSTANCE}"
  gcloud compute disks snapshot --snapshot-names="${image}" "${INSTANCE}"
  echo "  Create disk from ${image} snapshot"
  gcloud compute disks create --source-snapshot="${image}" "${image}"
  echo "  Create image from ${image} image"
  gcloud compute images create "${image}" \
    --family="${family}" \
    --source-disk="${image}" \
    --description="Created by ${USER} for ${family} on $(date)"
  echo "  Delete ${image} disk"
  gcloud -q compute disks delete "${image}"
  echo "  Delete ${image} snapshot"
  gcloud -q compute snapshots delete "${image}"
}

create-agent() {
  echo "Create ${INSTANCE}..."
  check-kind
  gcloud compute instances create \
    "${INSTANCE}" \
    --description="created on $(date) by ${USER}" \
    --boot-disk-size="${DISK_SIZE}" \
    --boot-disk-type="${DISK_TYPE}" \
    "${IMAGE_FLAG}" \
    --machine-type="${MACHINE_TYPE}" \
    --scopes="${SCOPES}" \
    --tags='do-not-delete,jenkins'
  while ! gcloud compute ssh "${INSTANCE}" uname -a < /dev/null; do
    sleep 1
  done
}

copy-keys-agent() {
echo "Copying ssh keys to ${INSTANCE}..."
gcloud compute ssh "${MASTER}" << COPY_DONE
set -o errexit
sudo cp /var/lib/jenkins/gce_keys/google_compute_engine* .
sudo chown "${USER}:${USER}" google_compute_engine*
COPY_DONE
gcloud compute copy-files "${MASTER}:google_compute_engine*" .
gcloud compute copy-files google_compute_engine* "${INSTANCE}:."
gcloud compute ssh "${INSTANCE}" << PLACE_DONE
set -o errexit
sudo cp google_compute_engine* /var/lib/jenkins/gce_keys/
sudo cp /var/lib/jenkins/gce_keys/google_compute_engine* /home/jenkins/.ssh/
sudo chown jenkins:jenkins {/var/lib/jenkins/gce_keys,/home/jenkins/.ssh}/google_compute_engine{,.pub}
sudo su -c 'gcloud compute config-ssh' jenkins
PLACE_DONE
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
sudo mkdir -p /var/lib/jenkins/gce_keys /home/jenkins/.ssh
sudo chown -R jenkins:jenkins /var/lib/jenkins /home/jenkins/.ssh

# Update/upgrade
sudo apt-get -y update
sudo apt-get -y upgrade
INSTANTIATE_DONE
}

reboot-agent() {
  echo "Rebooting ${INSTANCE}..."
  gcloud compute ssh "${INSTANCE}" sudo reboot || gcloud compute instances reset "${INSTANCE}"
  sleep 30  # TODO(fejta): still but sightly less lame
  while ! gcloud compute ssh "${INSTANCE}" uname -a < /dev/null; do
    sleep 1
  done
}

if [[ -z "${1:-}" ]]; then
  auto-agent
fi

while [[ -n "${1:-}" ]]; do
  "${1}-agent"
  shift
done
