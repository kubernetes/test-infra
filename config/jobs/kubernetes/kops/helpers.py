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

import os
import zlib

# We support rapid focus on a few tests of high concern
# This should be used for temporary tests we are evaluating,
# and ideally linked to a bug, and removed once the bug is fixed
run_hourly = [
]

run_daily = [
]

script_dir = os.path.dirname(os.path.realpath(__file__))

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
    if k8s_version == 'ci':
        return False
    if kops_version is None:
        return False
    if k8s_version == 'master':
        return False
    if k8s_version is None:
        return True
    return float(k8s_version) > float(kops_version)

def k8s_version_info(k8s_version):
    test_package_url = ''
    test_package_dir = ''
    if k8s_version == 'ci':
        marker = 'latest.txt'
        k8s_deploy_url = "https://dl.k8s.io/ci/latest.txt"
        test_package_url = 'https://dl.k8s.io'
        test_package_dir = 'ci'
    elif k8s_version == 'stable':
        marker = 'stable.txt'
        k8s_deploy_url = "https://dl.k8s.io/release/stable.txt"
    elif k8s_version:
        marker = f"latest-{k8s_version}.txt"
        k8s_deploy_url = f"https://dl.k8s.io/ci/latest-{k8s_version}.txt" # pylint: disable=line-too-long
        test_package_url = 'https://dl.k8s.io'
        test_package_dir = 'ci'
    else:
        raise Exception('missing required k8s_version')
    return marker, k8s_deploy_url, test_package_url, test_package_dir

def create_args(kops_channel, networking, extra_flags, kops_image):
    args = f"--channel={kops_channel} --networking=" + networking

    image_overridden = False
    if extra_flags:
        for arg in extra_flags:
            if "--image=" in arg:
                image_overridden = True
            args = args + " " + arg
    if kops_image and not image_overridden:
        args = f"--image='{kops_image}' {args}"
    return args.strip()

def distro_shortener(distro):
    return distro.replace('ubuntuminimal', 'umini').replace('ubuntu', 'u').replace('debian', 'deb').replace('amazonlinux', 'amzn') # pylint: disable=line-too-long

# The pin file contains a list of key=value pairs, that holds images we want to pin.
# This enables us to use the latest image without fetching them from AWS every time.
def pinned_file():
    return os.path.join(script_dir, 'pinned.list')

# get_pinned returns the pinned value for the given key, or None if the key is not found
def get_pinned(key):
    # Read pinned file, which is a list of key=value pairs
    # If the key is not found, return None
    # Ignore if the file is not found
    try:
        with open(pinned_file(), 'r') as f:
            s = f.read().strip()
        for line in s.split('\n'):
            k, v = line.split('=', 1)
            if k == key:
                return v
        return None
    except FileNotFoundError:
        return None

# set_pinned appends a key=value pair to the pinned file
def set_pinned(key, value):
    # Append to the pinned file, which is a list of key=value pairs
    with open(pinned_file(), 'a') as f:
        f.write(f"{key}={value}\n")

# latest_aws_image returns the latest AWS image for the given owner, name, and arch
# If the image is pinned, it returns the pinned image
# Otherwise, it fetches the latest image from AWS and pins it
def latest_aws_image(owner, name, arch='x86_64'):
    pin = "aws://images/" + owner + "/" + name + ":" + arch
    image = get_pinned(pin)
    if image:
        return image

    import boto3 # pylint: disable=import-error, import-outside-toplevel

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
                    arch
                ],
            },
        ],
    )
    images = []
    for image in response['Images']:
        images.append(image['ImageLocation'].replace('amazon', owner))
    images.sort(reverse=True)
    image = images[0]
    set_pinned(pin, image)
    return image

# latest_gce_image returns the latest GCE image for the given project, family, and arch
# If the image is pinned, it returns the pinned image
# Otherwise, it fetches the latest image from the specified family
def latest_gce_image(project, family, arch="X86_64"):
    pin = f"gce://images/{project}/{family}:{arch}"
    image = get_pinned(pin)
    if image:
        return image

    from google.cloud import compute_v1  # pylint: disable=import-error, import-outside-toplevel
    from google.api_core.exceptions import NotFound  # pylint: disable=import-error, import-outside-toplevel

    client = compute_v1.ImagesClient()
    try:
        image = client.get_from_family(project=project, family=family)
    except NotFound:
        print(f"Image family {family}, has been deprecated by Google, please remove this OS from testing") # pylint: disable=line-too-long
        return ""

    if arch not in image.architecture:
        raise RuntimeError(
            f"Image family {family} has architecture {image.architecture} "
            f"which doesn't match requested {arch}"
        )

    image_name = f"{project}/{image.name}"
    set_pinned(pin, image_name)
    return image_name

# latest_azure_image returns the latest Azure image for the given publisher, offer, sku, and arch
# If the image is pinned, it returns the pinned image
# Otherwise, it fetches the latest image from Azure
def latest_azure_image(publisher, offer, sku, arch="X86_64"):
    pin = f"azure://images/{publisher}/{offer}/{sku}:{arch}"
    image = get_pinned(pin)
    if image:
        return image

    image_name = ""
    # for now, we just return the image from this hardcoded list
    # az vm image show --urn Canonical:ubuntu-24_04-lts:server:latest | jq .name
    if publisher == 'Canonical' and offer == 'ubuntu-24_04-lts':
        if arch == 'arm64':
            image_name = "Canonical:ubuntu-24_04-lts:server-arm64:24.04.202512100"
        else:
            image_name = "Canonical:ubuntu-24_04-lts:server:24.04.202512100"

    # az vm image show --urn Debian:debian-13:13-gen2:latest | jq .name
    elif publisher == 'Debian' and offer == 'debian-13':
        if arch == 'arm64':
            image_name = "Debian:debian-13:13-arm64:0.20251117.2299"
        else:
            image_name = "Debian:debian-13:13-gen2:0.20251117.2299"
    set_pinned(pin, image_name)
    return image_name

# Get latest images from some public images families
azure_distro_images = {
    'u2404': latest_azure_image('Canonical', 'ubuntu-24_04-lts', 'server'),
    'u2404arm64': latest_azure_image('Canonical', 'ubuntu-24_04-lts', 'server-arm64', 'arm64'),
    'deb13': latest_azure_image('Debian', 'debian-13', '13-gen2'),
}

gce_distro_images = {
    "deb12": latest_gce_image("debian-cloud", "debian-12"),
    "deb12arm64": latest_gce_image("debian-cloud", "debian-12-arm64", "ARM64"),
    "deb13": latest_gce_image("debian-cloud", "debian-13"),
    "deb13arm64": latest_gce_image("debian-cloud", "debian-13-arm64", "ARM64"),
    "u2204": latest_gce_image("ubuntu-os-cloud", "ubuntu-2204-lts"),
    "u2404": latest_gce_image("ubuntu-os-cloud", "ubuntu-2404-lts-amd64"),
    "u2404arm64": latest_gce_image("ubuntu-os-cloud", "ubuntu-2404-lts-arm64", "ARM64"),
    "umini2404": latest_gce_image("ubuntu-os-cloud", "ubuntu-minimal-2404-lts-amd64"),
    "umini2404arm64": latest_gce_image("ubuntu-os-cloud", "ubuntu-minimal-2404-lts-arm64", "ARM64"),
    "cos121": latest_gce_image("cos-cloud", "cos-121-lts"),
    "cos121arm64": latest_gce_image("cos-cloud", "cos-arm64-121-lts", "ARM64"),
    "cos125": latest_gce_image("cos-cloud", "cos-125-lts"),
    "cos125arm64": latest_gce_image("cos-cloud", "cos-arm64-125-lts", "ARM64"),
    "cosdev": latest_gce_image("cos-cloud", "cos-dev"),
    "cosdevarm64": latest_gce_image("cos-cloud", "cos-arm64-dev", "ARM64"),
    "rocky10": latest_gce_image("rocky-linux-cloud", "rocky-linux-10-optimized-gcp"),
    "rocky10arm64": latest_gce_image("rocky-linux-cloud", "rocky-linux-10-optimized-gcp-arm64", "ARM64"), # pylint: disable=line-too-long
    "rhel10": latest_gce_image("rhel-cloud", "rhel-10"),
    "rhel10arm64": latest_gce_image("rhel-cloud", "rhel-10-arm64", "ARM64"),
    "fedora43": latest_gce_image("fedora-cloud", "fedora-cloud-43-x86-64"),
}

aws_distro_images = {
    'al2023': latest_aws_image('137112412989', 'al2023-ami-2*-kernel-6.12-x86_64'),
    'al2023arm64': latest_aws_image('137112412989', 'al2023-ami-2*-kernel-6.12-arm64', 'arm64'),
    'amzn2': latest_aws_image('137112412989', 'amzn2-ami-kernel-5.10-hvm-*-x86_64-gp2'),
    'deb11': latest_aws_image('136693071363', 'debian-11-amd64-*'),
    'deb12': latest_aws_image('136693071363', 'debian-12-amd64-*'),
    'deb13': latest_aws_image('136693071363', 'debian-13-amd64-*'),
    'deb13arm64': latest_aws_image('136693071363', 'debian-13-arm64-*', 'arm64'),
    'flatcar': latest_aws_image('075585003325', 'Flatcar-alpha-*-hvm'),
    'flatcararm64': latest_aws_image('075585003325', 'Flatcar-alpha-*-hvm', 'arm64'),
    'rhel9': latest_aws_image('309956199498', 'RHEL-9.*_HVM-*-x86_64-*'),
    'rhel10arm64': latest_aws_image('309956199498', 'RHEL-10.*_HVM-*-arm64-*', 'arm64'),
    'rocky9': latest_aws_image('792107900819', 'Rocky-9-EC2-Base-9.*.x86_64'),
    'rocky10arm64': latest_aws_image('792107900819', 'Rocky-10-EC2-Base-10.*.aarch64', 'arm64'),
    'u2204': latest_aws_image('099720109477', 'ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-amd64-server-*'), # pylint: disable=line-too-long
    'u2204arm64': latest_aws_image('099720109477', 'ubuntu/images/hvm-ssd/ubuntu-jammy-22.04-arm64-server-*', 'arm64'), # pylint: disable=line-too-long
    'u2404': latest_aws_image('099720109477', 'ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-amd64-server-*'), # pylint: disable=line-too-long
    'u2404arm64': latest_aws_image('099720109477', 'ubuntu/images/hvm-ssd-gp3/ubuntu-noble-24.04-arm64-server-*', 'arm64'), # pylint: disable=line-too-long
    'u2510': latest_aws_image('099720109477', 'ubuntu/images/hvm-ssd-gp3/ubuntu-questing-25.10-amd64-server-*'), # pylint: disable=line-too-long
    'u2510arm64': latest_aws_image('099720109477', 'ubuntu/images/hvm-ssd-gp3/ubuntu-questing-25.10-arm64-server-*', 'arm64'), # pylint: disable=line-too-long
}

aws_distros_ssh_user = {
    'al2023': 'ec2-user',
    'al2023arm64': 'ec2-user',
    'amzn2': 'ec2-user',
    'deb11': 'admin',
    'deb12': 'admin',
    'deb13': 'admin',
    'deb13arm64': 'admin',
    'flatcar': 'core',
    'flatcararm64': 'core',
    'rhel9': 'ec2-user',
    'rhel10arm64': 'ec2-user',
    'rocky9': 'rocky',
    'rocky10arm64': 'rocky',
    'u2004': 'ubuntu',
    'u2004arm64': 'ubuntu',
    'u2204': 'ubuntu',
    'u2204arm64': 'ubuntu',
    'u2404': 'ubuntu',
    'u2404arm64': 'ubuntu',
    'u2510': 'ubuntu',
    'u2510arm64': 'ubuntu',
}
