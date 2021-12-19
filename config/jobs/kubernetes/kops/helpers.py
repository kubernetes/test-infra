# Copyright 2020 The Kubernetes Authors.
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

import zlib

import boto3 # pylint: disable=import-error

# We support rapid focus on a few tests of high concern
# This should be used for temporary tests we are evaluating,
# and ideally linked to a bug, and removed once the bug is fixed
run_hourly = [
]

run_daily = [
    'kops-grid-scenario-service-account-iam',
    'kops-grid-scenario-arm64',
    'kops-grid-scenario-serial-test-for-timeout',
    'kops-grid-scenario-terraform',
]

def simple_hash(s):
    # & 0xffffffff avoids python2/python3 compatibility
    return zlib.crc32(s.encode()) & 0xffffffff

def build_cron(key, runs_per_day):
    runs_per_week = 0
    minute = simple_hash("minutes:" + key) % 60
    hour = simple_hash("hours:" + key) % 24
    day_of_week = simple_hash("day_of_week:" + key) % 7

    if runs_per_day > 0:
        hour_denominator = 24 / runs_per_day
        hour_offset = simple_hash("hours:" + key) % hour_denominator
        return "%d %d-23/%d * * *" % (minute, hour_offset, hour_denominator), (runs_per_day * 7)

    # run Ubuntu 20.04 (Focal) jobs more frequently
    if "u2004" in key:
        runs_per_week += 7
        return "%d %d * * *" % (minute, hour), runs_per_week

    # run hotlist jobs more frequently
    if key in run_hourly:
        runs_per_week += 24 * 7
        return "%d * * * *" % (minute), runs_per_week

    if key in run_daily:
        runs_per_week += 7
        return "%d %d * * *" % (minute, hour), runs_per_week

    runs_per_week += 1
    return "%d %d * * %d" % (minute, hour, day_of_week), runs_per_week

def replace_or_remove_line(s, pattern, new_str):
    keep = []
    for line in s.split('\n'):
        if pattern in line:
            if new_str:
                line = line.replace(pattern, new_str)
                keep.append(line)
        else:
            keep.append(line)
    return '\n'.join(keep)

def should_skip_newer_k8s(k8s_version, kops_version):
    if kops_version is None:
        return False
    if k8s_version is None:
        return True
    return float(k8s_version) > float(kops_version)

def k8s_version_info(k8s_version):
    test_package_bucket = ''
    test_package_dir = ''
    if k8s_version == 'latest':
        marker = 'latest.txt'
        k8s_deploy_url = "https://storage.googleapis.com/kubernetes-release/release/latest.txt"
    elif k8s_version == 'ci':
        marker = 'latest.txt'
        k8s_deploy_url = "https://storage.googleapis.com/k8s-release-dev/ci/latest.txt"
        test_package_bucket = 'k8s-release-dev'
        test_package_dir = 'ci'
    elif k8s_version == 'stable':
        marker = 'stable.txt'
        k8s_deploy_url = "https://storage.googleapis.com/kubernetes-release/release/stable.txt"
    elif k8s_version:
        marker = f"stable-{k8s_version}.txt"
        k8s_deploy_url = f"https://storage.googleapis.com/kubernetes-release/release/stable-{k8s_version}.txt" # pylint: disable=line-too-long
    else:
        raise Exception('missing required k8s_version')
    return marker, k8s_deploy_url, test_package_bucket, test_package_dir

def create_args(kops_channel, networking, container_runtime, extra_flags, kops_image):
    args = f"--channel={kops_channel} --networking=" + networking
    if container_runtime:
        args += f" --container-runtime={container_runtime}"

    if kops_image:
        image_overridden = False
        if extra_flags:
            for arg in extra_flags:
                if "--image=" in arg:
                    image_overridden = True
                args = args + " " + arg
        if not image_overridden:
            args = f"--image='{kops_image}' {args}"
    return args.strip()

def latest_aws_image(owner, name, arm64=False):
    client = boto3.client('ec2', region_name='us-east-1')
    response = client.describe_images(
        Owners=[owner],
        Filters=[
            {
                'Name': 'name',
                'Values': [
                    name,
                ],
            },
            {
                'Name': 'architecture',
                'Values': [
                    'arm64' if arm64 else 'x86_64',
                ],
            },
        ],
    )
    images = {}
    for image in response['Images']:
        images[image['CreationDate']] = image['ImageLocation']
    return images[sorted(images, reverse=True)[0]]

distro_images = {
    'amzn2': latest_aws_image('137112412989', 'amzn2-ami-kernel-5.10-hvm-*-x86_64-gp2'),
    'deb9': latest_aws_image('379101102735', 'debian-stretch-hvm-x86_64-gp2-*'),
    'deb10': latest_aws_image('136693071363', 'debian-10-amd64-*'),
    'deb11': latest_aws_image('136693071363', 'debian-11-amd64-*'),
    'flatcar': latest_aws_image('075585003325', 'Flatcar-stable-*-hvm'),
    'rhel7': latest_aws_image('309956199498', 'RHEL-7.*_HVM_*-x86_64-0-Hourly2-GP2'),
    'rhel8': latest_aws_image('309956199498', 'RHEL-8.4.*_HVM-*-x86_64-0-Hourly2-GP2'),
    'u1804': latest_aws_image('099720109477', 'ubuntu/images/hvm-ssd/ubuntu-bionic-18.04-amd64-server-*'), # pylint: disable=line-too-long
    'u2004': latest_aws_image('099720109477', 'ubuntu/images/hvm-ssd/ubuntu-focal-20.04-amd64-server-*'), # pylint: disable=line-too-long
    'u2004arm64': latest_aws_image('099720109477', 'ubuntu/images/hvm-ssd/ubuntu-focal-20.04-arm64-server-*', True), # pylint: disable=line-too-long
    'u2110': latest_aws_image('099720109477', 'ubuntu/images/hvm-ssd/ubuntu-impish-21.10-amd64-server-*'), # pylint: disable=line-too-long
    'u2204': latest_aws_image('099720109477', 'ubuntu/images-testing/hvm-ssd/ubuntu-jammy-daily-amd64-server-*'), # pylint: disable=line-too-long
}

distros_ssh_user = {
    'amzn2': 'ec2-user',
    'deb9': 'admin',
    'deb10': 'admin',
    'deb11': 'admin',
    'flatcar': 'core',
    'rhel7': 'ec2-user',
    'rhel8': 'ec2-user',
    'u1804': 'ubuntu',
    'u2004': 'ubuntu',
    'u2004arm64': 'ubuntu',
    'u2110': 'ubuntu',
    'u2204': 'ubuntu',
}
