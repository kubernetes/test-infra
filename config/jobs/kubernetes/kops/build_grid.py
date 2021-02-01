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
- name: {{job_name}}
  interval: '{{interval}}'
  cron: '{{cron}}'
  labels:
    preset-service-account: "true"
    preset-aws-ssh: "true"
    preset-aws-credential: "true"
  decorate: true
  decoration_config:
    timeout: {{job_timeout}}
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
      - --ginkgo-parallel={{test_parallelism}}
      - --kops-args={{kops_args}}
      - --kops-feature-flags={{kops_feature_flags}}
      - --kops-image={{kops_image}}
      - --kops-priority-path=/workspace/kubernetes/platforms/linux/amd64
      - --kops-version={{kops_deploy_url}}
      - --kops-zones={{kops_zones}}
      - --provider=aws
      - --test_args={{test_args}}
      - --timeout={{test_timeout}}
      image: {{e2e_image}}
      imagePullPolicy: Always
      resources:
        limits:
          memory: 2Gi
        requests:
          cpu: "2"
          memory: 2Gi
"""

kubetest2_template = """
- name: {{job_name}}
  cron: '{{cron}}'
  interval: '{{interval}}'
  labels:
    preset-service-account: "true"
    preset-aws-ssh: "true"
    preset-aws-credential: "true"
  decorate: true
  decoration_config:
    timeout: {{job_timeout}}
  extra_refs:
  - org: kubernetes
    repo: kops
    base_ref: master
    workdir: true
    path_alias: k8s.io/kops
  spec:
    containers:
    - command:
      - runner.sh
      args:
      - bash
      - -c
      - |
        make test-e2e-install
        kubetest2 kops \\
          -v 2 \\
          --up --down \\
          --cloud-provider=aws \\
          --create-args="--image={{kops_image}} --networking={{networking}} --container-runtime={{container_runtime}}" \\
          --env=KOPS_FEATURE_FLAGS={{kops_feature_flags}} \\
          --kops-version-marker={{kops_deploy_url}} \\
          --kubernetes-version={{k8s_deploy_url}} \\
          --test=kops \\
          -- \\
          --test-package-marker={{marker}} \\
          --parallel {{test_parallelism}} \\
          --skip-regex="{{skip_regex}}"
      env:
      - name: KUBE_SSH_KEY_PATH
        value: /etc/aws-ssh/aws-ssh-private
      - name: KUBE_SSH_USER
        value: {{kops_ssh_user}}
      image: {{e2e_image}}
      imagePullPolicy: Always
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
    'kops-grid-scenario-arm64',
    'kops-grid-scenario-aws-cloud-controller-manager',
    'kops-grid-scenario-serial-test-for-timeout',
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
    found = False
    for line in s.split('\n'):
        trimmed = line.strip()
        if trimmed.startswith(prefix):
            found = True
        else:
            keep.append(line)
    if not found:
        raise Exception("line not found with prefix: " + prefix)
    return '\n'.join(keep)

def should_skip_newer_k8s(k8s_version, kops_version):
    if kops_version is None:
        return False
    if k8s_version is None:
        return True
    return float(k8s_version) > float(kops_version)

def build_test(cloud='aws',
               distro=None,
               networking=None,
               container_runtime=None,
               k8s_version='latest',
               kops_version=None,
               kops_zones=None,
               name_override=None,
               feature_flags=(),
               extra_flags=None,
               extra_dashboards=None,
               interval=None,
               test_parallelism=25,
               test_timeout_minutes=60):
    # pylint: disable=too-many-statements,too-many-branches,too-many-arguments

    if container_runtime == "containerd" and (kops_version == "1.18" or networking in (None, "kopeio")): # pylint: disable=line-too-long
        return
    if should_skip_newer_k8s(k8s_version, kops_version):
        return

    if distro is None:
        kops_ssh_user = 'ubuntu'
        kops_image = '099720109477/ubuntu/images/hvm-ssd/ubuntu-focal-20.04-amd64-server-20210119.1'
    elif distro == 'amzn2':
        kops_ssh_user = 'ec2-user'
        kops_image = '137112412989/amzn2-ami-hvm-2.0.20201126.0-x86_64-gp2'
    elif distro == 'centos7':
        kops_ssh_user = 'centos'
        kops_image = "125523088429/CentOS 7.9.2009 x86_64"
    elif distro == 'centos8':
        kops_ssh_user = 'centos'
        kops_image = "125523088429/CentOS 8.3.2011 x86_64"
    elif distro == 'deb9':
        kops_ssh_user = 'admin'
        kops_image = '379101102735/debian-stretch-hvm-x86_64-gp2-2020-10-31-2842'
    elif distro == 'deb10':
        kops_ssh_user = 'admin'
        kops_image = '136693071363/debian-10-amd64-20201207-477'
    elif distro == 'flatcar':
        kops_ssh_user = 'core'
        kops_image = '075585003325/Flatcar-stable-2605.11.0-hvm'
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

    if k8s_version == 'latest':
        extract = "release/latest"
        marker = 'latest.txt'
        k8s_deploy_url = "https://storage.googleapis.com/kubernetes-release/release/latest.txt"
        e2e_image = "gcr.io/k8s-testimages/kubekins-e2e:v20210204-d375b29-master"
    elif k8s_version == 'stable':
        extract = "release/stable"
        marker = 'stable.txt'
        k8s_deploy_url = "https://storage.googleapis.com/kubernetes-release/release/stable.txt"
        e2e_image = "gcr.io/k8s-testimages/kubekins-e2e:v20210204-d375b29-master"
    elif k8s_version:
        extract = expand("release/stable-{k8s_version}")
        marker = expand("stable-{k8s_version}.txt")
        k8s_deploy_url = expand("https://storage.googleapis.com/kubernetes-release/release/stable-{k8s_version}.txt") # pylint: disable=line-too-long
        # Hack to stop the autobumper getting confused
        e2e_image = "gcr.io/k8s-testimages/kubekins-e2e:v20210204-d375b29-1.18"
        e2e_image = e2e_image[:-4] + k8s_version
    else:
        raise Exception('missing required k8s_version')

    kops_args = ""
    if networking:
        kops_args = kops_args + " --networking=" + networking

    if container_runtime:
        kops_args = kops_args + " --container-runtime=" + container_runtime

    if extra_flags:
        for arg in extra_flags:
            kops_args = kops_args + " " + arg

    kops_args = kops_args.strip()

    skip_regex = r'\[Slow\]|\[Serial\]|\[Disruptive\]|\[Flaky\]|\[Feature:.+\]|\[HPA\]|Dashboard|RuntimeClass|RuntimeHandler|Services.*functioning.*NodePort|Services.*rejected.*endpoints|Services.*affinity' # pylint: disable=line-too-long
    if networking == "cilium":
        # https://github.com/cilium/cilium/issues/10002
        skip_regex += r'|TCP.CLOSE_WAIT'
    test_args = r'--ginkgo.skip=' + skip_regex

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

    # We current have an issue with long cluster names; let's hash and warn if we encounter them
    cluster_name = "e2e-kops" + suffix
    if name_override:
        cluster_name = name_override
    if len(cluster_name) > 32:
        md5 = hashlib.md5(cluster_name.encode('utf-8'))
        cluster_name = cluster_name[0:20] + "--" + md5.hexdigest()[0:10]
    cluster_name += ".test-cncf-aws.k8s.io"

    if len(cluster_name) > 53:
        raise Exception("cluster name %s is probably too long" % (cluster_name))

    tab = 'kops-grid' + suffix

    if name_override:
        tab = name_override

    if tab in skip_jobs:
        return
    job_name = 'e2e-' + tab

    cron = build_cron(tab)

    # As kubetest2 adds support for additional configurations we can reduce this conditional
    # and migrate more of the grid jobs to kubetest2
    use_kubetest2 = container_runtime == 'containerd' and distro == 'u2004' and \
        len(feature_flags) == 0 and extra_flags is None and kops_zones is None

    y = template
    if use_kubetest2:
        y = kubetest2_template
    y = y.replace('{{cluster_name}}', cluster_name)
    y = y.replace('{{suffix}}', suffix)
    y = y.replace('{{job_name}}', job_name)
    y = y.replace('{{kops_ssh_user}}', kops_ssh_user)
    y = y.replace('{{kops_args}}', kops_args)
    y = y.replace('{{test_args}}', test_args)
    if interval:
        y = y.replace('{{interval}}', interval)
        y = remove_line_with_prefix(y, 'cron: ')
    else:
        y = y.replace('{{cron}}', cron)
        y = remove_line_with_prefix(y, 'interval: ')
    y = y.replace('{{k8s_deploy_url}}', k8s_deploy_url)
    y = y.replace('{{kops_deploy_url}}', kops_deploy_url)
    y = y.replace('{{extract}}', extract)
    y = y.replace('{{e2e_image}}', e2e_image)
    y = y.replace('{{kops_image}}', kops_image)

    y = y.replace('{{test_parallelism}}', str(test_parallelism))
    y = y.replace('{{job_timeout}}', str(test_timeout_minutes + 30) + 'm')
    y = y.replace('{{test_timeout}}', str(test_timeout_minutes) + 'm')

    # specific to kubetest2
    if use_kubetest2:
        if networking:
            y = y.replace('{{networking}}', networking)
        else:
            y = remove_line_with_prefix(y, "--networking=")
        y = y.replace('{{marker}}', marker)
        y = y.replace('{{skip_regex}}', skip_regex)
        y = y.replace('{{container_runtime}}', container_runtime)
        y = y.replace('{{kops_feature_flags}}', ','.join(feature_flags))

    else:
        if kops_zones:
            y = y.replace('{{kops_zones}}', ','.join(kops_zones))
        else:
            y = remove_line_with_prefix(y, "- --kops-zones=")

        if feature_flags:
            y = y.replace('{{kops_feature_flags}}', ','.join(feature_flags))
        else:
            y = remove_line_with_prefix(y, "- --kops-feature-flags=")


    if kops_version:
        y = y.replace('{{kops_version}}', kops_version)
    else:
        y = y.replace('{{kops_version}}', "latest")

    spec = {
        'cloud': cloud,
        'networking': networking,
        'distro': distro,
        'k8s_version': k8s_version,
        'kops_version': kops_version,
        'container_runtime': container_runtime,
    }
    if feature_flags:
        spec['feature_flags'] = ','.join(feature_flags)
    if extra_flags:
        spec['extra_flags'] = ' '.join(extra_flags)
    if kops_zones:
        spec['kops_zones'] = ','.join(kops_zones)
    jsonspec = json.dumps(spec, sort_keys=True)

    dashboards = [
        'sig-cluster-lifecycle-kops',
        'google-aws',
    ]

    if distro:
        dashboards.append('kops-distro-' + distro)
    else:
        dashboards.append('kops-distro-default')

    if k8s_version:
        dashboards.append('kops-k8s-' + k8s_version)
    else:
        dashboards.append('kops-k8s-latest')

    if kops_version:
        dashboards.append('kops-' + kops_version)
    else:
        dashboards.append('kops-latest')

    if extra_dashboards:
        dashboards.extend(extra_dashboards)

    if use_kubetest2:
        dashboards.append('kops-kubetest2')

    annotations = {
        'testgrid-dashboards': ', '.join(sorted(dashboards)),
        'testgrid-days-of-results': '90',
        'testgrid-tab-name': tab,
    }
    for (k, v) in spec.items():
        annotations['test.kops.k8s.io/' + k] = v if v else ""

    extra = yaml.dump({'annotations': annotations}, width=9999, default_flow_style=False)

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
    'deb9',
    'deb10',
    'flatcar',
    'rhel7',
    'rhel8',
    'u1804',
    'u2004',
]

k8s_versions = [
    #"latest", # disabled until we're ready to test 1.21
    "1.17",
    "1.18",
    "1.19",
    "1.20"
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
                                   extra_dashboards=['kops-grid'],
                                   k8s_version=k8s_version,
                                   kops_version=kops_version,
                                   networking=networking,
                                   container_runtime=container_runtime)

    # A one-off scenario testing arm64
    # TODO: Would be nice to default the arm image, perhaps based on the instance type
    build_test(name_override="kops-grid-scenario-arm64",
               cloud="aws",
               distro="u2004",
               kops_zones=['us-east-2b'],
               extra_flags=['--node-size=m6g.large',
                            '--master-size=m6g.large',
                            '--image=099720109477/ubuntu/images/hvm-ssd/ubuntu-focal-20.04-arm64-server-20210106'], # pylint: disable=line-too-long
               extra_dashboards=['kops-misc'])

    # A special test for JWKS
    build_test(name_override="kops-grid-scenario-public-jwks",
               cloud="aws",
               distro="u2004",
               feature_flags=["UseServiceAccountIAM", "PublicJWKS"],
               extra_flags=['--api-loadbalancer-type=public'],
               extra_dashboards=['kops-misc'])

    # A special test for AWS Cloud-Controller-Manager
    build_test(name_override="kops-grid-scenario-aws-cloud-controller-manager",
               cloud="aws",
               distro="u2004",
               k8s_version="1.19",
               feature_flags=["EnableExternalCloudController,SpecOverrideFlag"],
               extra_flags=['--override=cluster.spec.cloudControllerManager.cloudProvider=aws',
                            '--override=cluster.spec.cloudConfig.awsEBSCSIDriver.enabled=true'],
               extra_dashboards=['provider-aws-cloud-provider-aws', 'kops-misc'])

    # A special test to diagnose test timeouts
    # cf https://github.com/kubernetes/test-infra/issues/20738
    build_test(force_name="scenario-serial-test-for-timeout",
               cloud="aws",
               networking="calico",
               distro="amzn2",
               k8s_version="1.20",
               test_parallelism=1,
               test_timeout_minutes=300)

    print("")
    print("# %d jobs, total of %d runs per week" % (job_count, runs_per_week))

if __name__ == "__main__":
    generate()
