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

import hashlib
import json
import zlib

import yaml

template = """
- name: e2e-kops-grid{{suffix}}
  cron: '{{cron}}'
  labels:
    preset-service-account: "true"
    preset-aws-ssh: "true"
    preset-aws-credential: "true"
  decorate: true
  decoration_config:
    timeout: 90m
  spec:
    containers:
    - command:
      - runner.sh
      - /workspace/scenarios/kubernetes_e2e.py
      args:
      - --cluster={{cluster_name}}
      - --deployment=kops
      - --kops-ssh-user={{kops_ssh_user}}
      - --env=KUBE_SSH_USER={{kops_ssh_user}}
      - --env=KOPS_DEPLOY_LATEST_URL={{k8s_deploy_url}}
      - --env=KOPS_KUBE_RELEASE_URL=https://storage.googleapis.com/kubernetes-release/release
      - --env=KOPS_RUN_TOO_NEW_VERSION=1
      - --extract={{extract}}
      - --ginkgo-parallel
      - --kops-args={{kops_args}}
      - --kops-feature-flags={{kops_feature_flags}}
      - --kops-image={{kops_image}}
      - --kops-priority-path=/workspace/kubernetes/platforms/linux/amd64
      - --kops-version={{kops_deploy_url}}
      - --provider=aws
      - --test_args={{test_args}}
      - --timeout=60m
      image: {{e2e_image}}
      resources:
        limits:
          memory: 2Gi
        requests:
          cpu: "2"
          memory: 2Gi
"""

# We support rapid focus on a few tests of high concern
# This should be used for temporary tests we are evaluating,
# and ideally linked to a bug, and removed once the bug is fixed
run_hourly = [
]

run_daily = [
    'kops-grid-scenario-public-jwks',
]

# These are job tab names of unsupported grid combinations
skip_jobs = [
    # https://github.com/cilium/cilium/blob/71cfb265d53b63a2be3806fb3fd4425fa36262ff/Documentation/install/system_requirements.rst#centos-foot
    'kops-grid-cilium-amzn2',
    'kops-grid-cilium-amzn2-k18',
    'kops-grid-cilium-centos7',
    'kops-grid-cilium-centos7-k17',
    'kops-grid-cilium-centos7-k17-ko19',
    'kops-grid-cilium-centos7-k18',
    'kops-grid-cilium-centos7-k18-ko19',
    'kops-grid-cilium-centos7-ko19',
    'kops-grid-cilium-deb9',
    'kops-grid-cilium-deb9-k18',
    'kops-grid-cilium-rhel7',
    'kops-grid-cilium-rhel7-k17',
    'kops-grid-cilium-rhel7-k17-ko19',
    'kops-grid-cilium-rhel7-k18',
    'kops-grid-cilium-rhel7-k18-ko19',
    'kops-grid-cilium-rhel7-ko19',
    'kops-grid-cilium-u1604',
    'kops-grid-cilium-u1604-k17',
    'kops-grid-cilium-u1604-k17-ko19',
    'kops-grid-cilium-u1604-k18',
    'kops-grid-cilium-u1604-k18-ko19',
    'kops-grid-cilium-u1604-ko19',
]

def simple_hash(s):
    # & 0xffffffff avoids python2/python3 compatibility
    return zlib.crc32(s.encode()) & 0xffffffff

runs_per_week = 0
job_count = 0

def build_cron(key):
    global job_count # pylint: disable=global-statement
    global runs_per_week # pylint: disable=global-statement

    minute = simple_hash("minutes:" + key) % 60
    hour = simple_hash("hours:" + key) % 24
    day_of_week = simple_hash("day_of_week:" + key) % 7

    job_count += 1

    # run Ubuntu 20.04 (Focal) jobs more frequently
    if "u2004" in key:
        runs_per_week += 7
        return "%d %d * * *" % (minute, hour)

    # run hotlist jobs more frequently
    if key in run_hourly:
        runs_per_week += 24 * 7
        return "%d * * * *" % (minute)

    if key in run_daily:
        runs_per_week += 7
        return "%d %d * * *" % (minute, hour)

    runs_per_week += 1
    return "%d %d * * %d" % (minute, hour, day_of_week)

def remove_line_with_prefix(s, prefix):
    keep = []
    for line in s.split('\n'):
        trimmed = line.strip()
        if trimmed.startswith(prefix):
            found = True
        else:
            keep.append(line)
    if not found:
        raise Exception("line not found with prefix: " + prefix)
    return '\n'.join(keep)

def build_test(cloud='aws',
               distro=None,
               networking=None,
               container_runtime=None,
               k8s_version=None,
               kops_version=None,
               force_name=None,
               feature_flags=None,
               extra_flags=None):
    # pylint: disable=too-many-statements,too-many-branches

    if distro is None:
        kops_ssh_user = 'ubuntu'
        kops_image = None
    elif distro == 'amzn2':
        kops_ssh_user = 'ec2-user'
        kops_image = '137112412989/amzn2-ami-hvm-2.0.20201126.0-x86_64-gp2'
    elif distro == 'centos7':
        kops_ssh_user = 'centos'
        kops_image = "679593333241/CentOS Linux 7 x86_64 HVM EBS ENA 2002_01-b7ee8a69-ee97-4a49-9e68-afaee216db2e-ami-0042af67f8e4dcc20.4" # pylint: disable=line-too-long
    elif distro == 'deb9':
        kops_ssh_user = 'admin'
        kops_image = '379101102735/debian-stretch-hvm-x86_64-gp2-2020-10-31-2842'
    elif distro == 'deb10':
        kops_ssh_user = 'admin'
        kops_image = '136693071363/debian-10-amd64-20201207-477'
    elif distro == 'flatcar':
        kops_ssh_user = 'core'
        kops_image = '075585003325/Flatcar-stable-2605.9.0-hvm'
    elif distro == 'u1604':
        kops_ssh_user = 'ubuntu'
        kops_image = '099720109477/ubuntu/images/hvm-ssd/ubuntu-xenial-16.04-amd64-server-20200429'
    elif distro == 'u1804':
        kops_ssh_user = 'ubuntu'
        kops_image = '099720109477/ubuntu/images/hvm-ssd/ubuntu-bionic-18.04-amd64-server-20201201'
    elif distro == 'u2004':
        kops_ssh_user = 'ubuntu'
        kops_image = '099720109477/ubuntu/images/hvm-ssd/ubuntu-focal-20.04-amd64-server-20201201'
    elif distro == 'rhel7':
        kops_ssh_user = 'ec2-user'
        kops_image = '309956199498/RHEL-7.9_HVM_GA-20200917-x86_64-0-Hourly2-GP2'
    elif distro == 'rhel8':
        kops_ssh_user = 'ec2-user'
        kops_image = '309956199498/RHEL-8.3.0_HVM-20201031-x86_64-0-Hourly2-GP2'
    else:
        raise Exception('unknown distro ' + distro)

    if container_runtime is None:
        container_runtime = 'docker'

    def expand(s):
        subs = {}
        if k8s_version:
            subs['k8s_version'] = k8s_version
        if kops_version:
            subs['kops_version'] = kops_version
        return s.format(**subs)

    if kops_version is None:
        # TODO: Move to kops-ci/markers/master/ once validated
        kops_deploy_url = "https://storage.googleapis.com/kops-ci/bin/latest-ci-updown-green.txt"
    else:
        kops_deploy_url = expand("https://storage.googleapis.com/kops-ci/markers/release-{kops_version}/latest-ci-updown-green.txt") # pylint: disable=line-too-long

    if k8s_version is None:
        extract = "release/latest"
        k8s_deploy_url = "https://storage.googleapis.com/kubernetes-release/release/latest.txt"
        e2e_image = "gcr.io/k8s-testimages/kubekins-e2e:v20201203-4778e22-master"
    else:
        extract = expand("release/stable-{k8s_version}")
        k8s_deploy_url = expand("https://storage.googleapis.com/kubernetes-release/release/stable-{k8s_version}.txt") # pylint: disable=line-too-long
        # Hack to stop the autobumper getting confused
        e2e_image = "gcr.io/k8s-testimages/kubekins-e2e:v20201203-4778e22-1.18"
        e2e_image = e2e_image[:-4] + k8s_version

    kops_args = ""
    if networking:
        kops_args = kops_args + " --networking=" + networking

    if container_runtime:
        kops_args = kops_args + " --container-runtime=" + container_runtime

    if extra_flags:
        for arg in extra_flags:
            kops_args = kops_args + " " + arg

    kops_args = kops_args.strip()

    test_args = r'--ginkgo.skip=\[Slow\]|\[Serial\]|\[Disruptive\]|\[Flaky\]|\[Feature:.+\]|\[HPA\]|Dashboard|Services.*functioning.*NodePort|Services.*rejected.*endpoints|Services.*affinity' # pylint: disable=line-too-long

    suffix = ""
    if cloud and cloud != "aws":
        suffix += "-" + cloud
    if networking:
        suffix += "-" + networking
    if distro:
        suffix += "-" + distro
    if k8s_version:
        suffix += "-k" + k8s_version.replace("1.", "")
    if kops_version:
        suffix += "-ko" + kops_version.replace("1.", "")
    if container_runtime:
        suffix += "-" + container_runtime

    if force_name:
        suffix = "-" + force_name

    # We current have an issue with long cluster names; let's hash and warn if we encounter them
    cluster_name = "e2e-kops" + suffix
    if len(cluster_name) > 32:
        md5 = hashlib.md5(cluster_name.encode('utf-8'))
        cluster_name = cluster_name[0:20] + "--" + md5.hexdigest()[0:10]
    cluster_name += ".test-cncf-aws.k8s.io"

    if len(cluster_name) > 53:
        raise Exception("cluster name %s is probably too long" % (cluster_name))

    tab = 'kops-grid' + suffix

    if tab in skip_jobs:
        return

    cron = build_cron(tab)

    y = template
    y = y.replace('{{cluster_name}}', cluster_name)
    y = y.replace('{{suffix}}', suffix)
    y = y.replace('{{kops_ssh_user}}', kops_ssh_user)
    y = y.replace('{{kops_args}}', kops_args)
    y = y.replace('{{test_args}}', test_args)
    y = y.replace('{{cron}}', cron)
    y = y.replace('{{k8s_deploy_url}}', k8s_deploy_url)
    y = y.replace('{{kops_deploy_url}}', kops_deploy_url)
    y = y.replace('{{extract}}', extract)
    y = y.replace('{{e2e_image}}', e2e_image)

    if kops_version:
        y = y.replace('{{kops_version}}', kops_version)
    else:
        y = y.replace('{{kops_version}}', "latest")

    if kops_image:
        y = y.replace('{{kops_image}}', kops_image)
    else:
        y = remove_line_with_prefix(y, "- --kops-image=")

    if feature_flags:
        y = y.replace('{{kops_feature_flags}}', ','.join(feature_flags))
    else:
        y = remove_line_with_prefix(y, "- --kops-feature-flags=")

    spec = {
        'cloud': cloud,
        'networking': networking,
        'distro': distro,
        'k8s_version': k8s_version,
        'kops_version': kops_version,
        'container_runtime': container_runtime,
    }
    if feature_flags:
        spec['feature_flags'] = feature_flags
    if extra_flags:
        spec['extra_flags'] = extra_flags
    jsonspec = json.dumps(spec, sort_keys=True)

    dashboards = [
        'sig-cluster-lifecycle-kops',
        'google-aws',
        'kops-grid',
    ]

    if distro:
        dashboards.append('kops-distro-' + distro)
    else:
        dashboards.append('kops-distro-default')

    if k8s_version:
        dashboards.append('kops-k8s-' + k8s_version)
    else:
        dashboards.append('kops-k8s-latest')

    annotations = {
        'testgrid-dashboards': ', '.join(dashboards),
        'testgrid-tab-name': tab,
    }

    extra = yaml.dump({'annotations': annotations}, width=9999)

    print("")
    print("# " + jsonspec)
    print(y.strip())
    for line in extra.splitlines():
        print("  " + line)

networking_options = [
    None,
    'calico',
    'cilium',
    'flannel',
    'kopeio',
]

distro_options = [
    'amzn2',
    'centos7',
    'deb9',
    'deb10',
    'flatcar',
    'rhel7',
    'rhel8',
    'u1604',
    'u1804',
    'u2004',
]

k8s_versions = [
    None,
    "1.17",
    "1.18",
    "1.19"
]

kops_versions = [
    None, # maps to latest
    "1.18",
    "1.19",
]

container_runtimes = [
    "docker",
    "containerd",
]

def generate():
    print("# Test scenarios generated by build-grid.py (do not manually edit)")
    print("periodics:")
    for container_runtime in container_runtimes:
        for networking in networking_options:
            for distro in distro_options:
                for k8s_version in k8s_versions:
                    for kops_version in kops_versions:
                        build_test(cloud="aws",
                                   distro=distro,
                                   k8s_version=k8s_version,
                                   kops_version=kops_version,
                                   networking=networking,
                                   container_runtime=container_runtime)

    # A one-off scenario testing arm64
    # TODO: Would be nice to default the arm image, perhaps based on the instance type
    build_test(force_name="scenario-arm64",
               cloud="aws",
               distro="u2004",
               extra_flags=['--zones=us-east-2b',
                            '--node-size=m6g.large',
                            '--master-size=m6g.large',
                            '--image=099720109477/ubuntu/images/hvm-ssd/ubuntu-focal-20.04-amd64-server-20201201']) # pylint: disable=line-too-long

    # A special test for JWKS
    build_test(force_name="scenario-public-jwks",
               cloud="aws",
               distro="u2004",
               feature_flags=["UseServiceAccountIAM", "PublicJWKS"],
               extra_flags=['--api-loadbalancer-type=public'])

    # A special test for AWS Cloud-Controller-Manager
    build_test(force_name="scenario-aws-cloud-controller-manager",
               cloud="aws",
               distro="u2004",
               feature_flags=["EnableExternalCloudController,SpecOverrideFlag"],
               extra_flags=['--override=cluster.spec.cloudControllerManager.cloudProvider=aws'])

    print("")
    print("# %d jobs, total of %d runs per week" % (job_count, runs_per_week))


generate()
