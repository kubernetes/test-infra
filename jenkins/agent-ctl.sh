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

# Run command with no args for usage instructions.

usage() {
  echo "Usage: $(basename "${0}") [--FLAGS] <INSTANCE> [ACTION ...]"
  echo 'Flags:'
  echo '  --base-image: use base-image instead of derived for instances'
  echo '  --fake: use tiny instance'
  echo '  --pr: talk to the pull-request instead of e2e server'
  echo '  --previous: use last known good CI image'
  echo '  --previous-pr: use last known good PR image'
  echo 'INSTANCE: the name of the instance to target'
  echo '  pr-: create a pr-builder-sized instance'
  echo '  light-: create an instance for light postcommit jobs (e2e)'
  echo '  heavy-: create an instance for heavy postcommit jobs (build)'
  echo 'Actions (auto by default):'
  echo '  attach: connect the instance to the jenkins master'
  echo '  auto: detach delete create attach'
  echo '  auto-image: delete create update copy-keys reboot update-image delete'
  echo '  copy-keys: copy ssh keys from master to agent'
  echo '  create: insert a new vm'
  echo '  create-image: create a new agent image'
  echo '  delete: delete a vm'
  echo '  detach: disconnect the instance from the jenkins master'
  echo '  reboot: reboot or hard reset the VM'
  echo '  update: configure prerequisite packages to run tests'
  echo '  update-image: update the image-family used to create new disks'
  echo 'Common commands:'
  echo '  # Refresh image'
  echo "  $(basename "${0}") --base-image light-agent auto-image"
  echo '  # Retire agent'
  echo "  $(basename "${0}") agent-heavy-666 detach delete"
  echo '  # Refresh agent'
  echo "  $(basename "${0}") agent-light-666"
  echo "  $(basename "${0}") --pr agent-pr-666"
  exit 1
}


set -o nounset
set -o errexit

GO_VERSION='go1.9.1.linux-amd64'
TIMEZONE='America/Los_Angeles'

FAKE=
PR=

# Defaults
BASE_IMAGE='debian-9'
IMAGE='jenkins-agent'
IMAGE_FLAG="--image-family=${IMAGE}"
IMAGE_PROJECT='kubernetes-jenkins'
SCOPES='cloud-platform,compute-rw,storage-full'  # TODO(fejta): verify

if [[ -z "${1:-}" ]]; then
  usage
fi

while true; do
  case "${1:-}" in
    --fake)
      FAKE=yes
      shift
      ;;
    --previous)
      # Currently jenkins-agent-20160926-0059
      IMAGE_FLAG='--image=jenkins-agent-20160613-2240'
      ;;
    --previous-pr)
      # Currently jenkins-agent-20160926-0000
      IMAGE_FLAG='--image=jenkins-agent-20160613-1431'
      PR=yes
      ;;
    --pr)
      PR=yes
      if [[ "${IMAGE_PROJECT}" == 'kubernetes-jenkins' ]]; then
        IMAGE_PROJECT='kubernetes-jenkins-pull'
      fi
      shift
      ;;
    --base-image)
      IMAGE_FLAG="--image-family=${BASE_IMAGE}"
      IMAGE_PROJECT='debian-cloud'
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
    # 6 agents
    # 1 executor, n1-standard-8, 150G pd-standard
    # Results:
    # load 14-32, 12G ram, 150 write IOPs, <20MB/s write
    DISK_SIZE='150GB'
    DISK_TYPE='pd-standard'
    MACHINE_TYPE='n1-standard-8'
    ;;
  pr)
    # Current experiment:
    # 1 executor, n1-standard-8, 200G pd
    # Results:
    # load 10-30, 10G ram
    DISK_SIZE='200GB'
    DISK_TYPE='pd-standard'
    MACHINE_TYPE='n1-standard-8'
    ;;
  *)
    ;;
esac

if [[ -n "${FAKE}" ]]; then
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
  detach-agent
  delete-agent
  create-agent
  attach-agent
}

tunnel-to-master() {
  if sudo netstat -anp | grep :8080 > /dev/null 2>&1 ; then
    sleep 1
  else
    echo "Please run gcloud compute ssh \"${MASTER}\" --ssh-flag='-L8080:localhost:8080'"
    exit 1
  fi
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
}


detach-agent() {
  echo "Detaching ${INSTANCE}..."
  master-change delete
}


attach-agent() {
  echo "Testing gcloud works on ${INSTANCE}..."
  gcloud compute ssh "${INSTANCE}" --command="gcloud compute instances list '--filter=name=${INSTANCE}'"
  echo "Checking presence of ssh keys on ${INSTANCE}..."
  gcloud compute ssh "${INSTANCE}" --command="[[ -f /var/lib/jenkins/gce_keys/google_compute_engine ]]"
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
    --image-project="${IMAGE_PROJECT}" \
    --machine-type="${MACHINE_TYPE}" \
    --scopes="${SCOPES}" \
    --tags='do-not-delete,jenkins'
  while ! gcloud compute ssh "${INSTANCE}" --command='uname -a' < /dev/null; do
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

sudo apt-get -y update
sudo apt-get -y install \
  apt-transport-https \
  ca-certificates \
  curl \
  gnupg2 \
  software-properties-common \
  python-openssl python-pyasn1 python-ndg-httpsclient \
  build-essential \
  tmpreaper \
  jq

# Install docker
curl -fsSL https://download.docker.com/linux/debian/gpg | sudo apt-key add -
sudo apt-key fingerprint 0EBFCD88 | grep '9DC8 5822 9FC7 DD38 854A  E2D8 8D81 803C 0EBF CD88'
sudo add-apt-repository -y "deb [arch=amd64] https://download.docker.com/linux/debian \$(lsb_release -cs) stable"
sudo apt-get -y update
sudo apt-get -y install docker-ce
sudo docker run hello-world
id jenkins || sudo useradd jenkins -m
sudo usermod -aG docker jenkins

# Use java8
sudo apt-get -y update
sudo apt-get -y install \
  openjdk-8-jdk
sudo update-alternatives --set java /usr/lib/jvm/java-8-openjdk-amd64/jre/bin/java
java -version 2>&1 | grep 1.8

# Install go (needed for hack/e2e.go)

wget "https://storage.googleapis.com/golang/${GO_VERSION}.tar.gz"
sudo tar xzvf "${GO_VERSION}.tar.gz" -C /usr/local


# Reboot on panic
sudo touch /etc/sysctl.conf
sudo sh -c 'cat << END >> /etc/sysctl.conf
kernel.panic_on_oops = 1
kernel.panic = 10
END'
sudo sysctl -p

# Keep tmp clean
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
  gcloud compute ssh "${INSTANCE}" --command='sudo reboot' || gcloud compute instances reset "${INSTANCE}"
  sleep 30  # TODO(fejta): still but sightly less lame
  while ! gcloud compute ssh "${INSTANCE}" --command='uname -a' < /dev/null; do
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
