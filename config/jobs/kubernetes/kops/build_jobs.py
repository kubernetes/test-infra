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
import math
import json
import re
import yaml
import jinja2 # pylint: disable=import-error


from helpers import ( # pylint: disable=import-error, no-name-in-module
    build_cron,
    create_args,
    distro_images,
    distros_ssh_user,
    k8s_version_info,
    should_skip_newer_k8s,
)

# These are job tab names of unsupported grid combinations
skip_jobs = [
]

image = "gcr.io/k8s-staging-test-infra/kubekins-e2e:v20230330-8e9af88c7d-master"

loader = jinja2.FileSystemLoader(searchpath="./templates")

##############
# Build Test #
##############

# Returns a string representing the periodic prow job and the number of job invocations per week
def build_test(cloud='aws',
               distro='u2204',
               networking='cilium',
               irsa=True,
               k8s_version='ci',
               kops_channel='alpha',
               kops_version=None,
               publish_version_marker=None,
               name_override=None,
               feature_flags=(),
               extra_flags=None,
               extra_dashboards=None,
               terraform_version=None,
               test_parallelism=25,
               test_timeout_minutes=60,
               skip_regex='',
               focus_regex=None,
               runs_per_day=0,
               scenario=None,
               env=None,
               template_path=None):
    # pylint: disable=too-many-statements,too-many-branches,too-many-arguments

    if kops_version is None:
        # TODO: Move to kops-ci/markers/master/ once validated
        kops_deploy_url = "https://storage.googleapis.com/kops-ci/bin/latest-ci-updown-green.txt"
    elif kops_version.startswith("https://"):
        kops_deploy_url = kops_version
        kops_version = None
    else:
        kops_deploy_url = f"https://storage.googleapis.com/kops-ci/markers/release-{kops_version}/latest-ci-updown-green.txt" # pylint: disable=line-too-long

    if should_skip_newer_k8s(k8s_version, kops_version):
        return None
    if networking == 'kopeio' and distro in ('flatcar', 'flatcararm64'):
        return None

    # Won't backport the rp_filter fixes to kops 1.23
    if networking == 'cilium' and distro == 'u2204'and kops_version == '1.23':
        return None

    if extra_flags is None:
        extra_flags = []

    if cloud == 'aws':
        kops_image = distro_images[distro]
        kops_ssh_user = distros_ssh_user[distro]
        kops_ssh_key_path = '/etc/aws-ssh/aws-ssh-private'

        if networking == 'cilium-eni':
            # Needed for higher "IPs per node" limits
            extra_flags.append('--node-size=t3.large')

    elif cloud == 'gce':
        kops_image = None
        kops_ssh_user = 'prow'
        kops_ssh_key_path = '/etc/ssh-key-secret/ssh-private'

    validation_wait = '20m' if distro in ('flatcar', 'flatcararm64') else None

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

    tab = name_override or (f"kops-grid{suffix}")
    job_name = f"e2e-{tab}"

    if irsa and cloud == "aws" and scenario is None:
        if extra_flags is None:
            extra_flags = []
        extra_flags.append("--discovery-store=s3://k8s-kops-prow/discovery")

    marker, k8s_deploy_url, test_package_bucket, test_package_dir = k8s_version_info(k8s_version)
    args = create_args(kops_channel, networking, extra_flags, kops_image)

    node_ig_overrides = ""
    cp_ig_overrides = ""
    # if distro == "flatcar":
    #     # https://github.com/flatcar-linux/Flatcar/issues/220
    #     node_ig_overrides += "spec.instanceMetadata.httpTokens=optional"
    #     cp_ig_overrides += "spec.instanceMetadata.httpTokens=optional"

    if tab in skip_jobs:
        return None

    cron, runs_per_week = build_cron(tab, runs_per_day)

    # Scenario-specific parameters
    if env is None:
        env = {}

    tmpl_file = "periodic.yaml.jinja"
    if scenario is not None:
        tmpl_file = "periodic-scenario.yaml.jinja"
        name_hash = hashlib.md5(job_name.encode()).hexdigest()
        env['CLOUD_PROVIDER'] = cloud
        env['CLUSTER_NAME'] = f"e2e-{name_hash[0:10]}-{name_hash[12:17]}.test-cncf-aws.k8s.io"
        env['KOPS_STATE_STORE'] = 's3://k8s-kops-prow'
        env['KUBE_SSH_USER'] = kops_ssh_user
        if extra_flags:
            env['KOPS_EXTRA_FLAGS'] = " ".join(extra_flags)
        if irsa and cloud == "aws":
            env['KOPS_IRSA'] = "true"

    tmpl = jinja2.Environment(loader=loader).get_template(tmpl_file)
    job = tmpl.render(
        job_name=job_name,
        cloud=cloud,
        cron=cron,
        kops_ssh_user=kops_ssh_user,
        kops_ssh_key_path=kops_ssh_key_path,
        create_args=args,
        cp_ig_overrides=cp_ig_overrides,
        node_ig_overrides=node_ig_overrides,
        k8s_deploy_url=k8s_deploy_url,
        kops_deploy_url=kops_deploy_url,
        test_parallelism=str(test_parallelism),
        job_timeout=str(test_timeout_minutes + 30) + 'm',
        test_timeout=str(test_timeout_minutes) + 'm',
        marker=marker,
        template_path=template_path,
        skip_regex=skip_regex,
        kops_feature_flags=','.join(feature_flags),
        terraform_version=terraform_version,
        test_package_bucket=test_package_bucket,
        test_package_dir=test_package_dir,
        focus_regex=focus_regex,
        publish_version_marker=publish_version_marker,
        validation_wait=validation_wait,
        image=image,
        scenario=scenario,
        env=env,
    )

    spec = {
        'cloud': cloud,
        'networking': networking,
        'distro': distro,
        'k8s_version': k8s_version,
        'kops_version': kops_version,
        'kops_channel': kops_channel,
    }
    if feature_flags:
        spec['feature_flags'] = ','.join(feature_flags)
    if extra_flags:
        spec['extra_flags'] = ' '.join(extra_flags)
    jsonspec = json.dumps(spec, sort_keys=True)

    dashboards = [
        'sig-cluster-lifecycle-kops',
        f"kops-distro-{distro.removesuffix('arm64')}",
        f"kops-k8s-{k8s_version or 'latest'}",
        f"kops-{kops_version or 'latest'}",
    ]
    if cloud == 'aws':
        dashboards.extend(['google-aws'])
    if cloud == 'gce':
        dashboards.extend(['kops-gce'])

    if extra_dashboards:
        dashboards.extend(extra_dashboards)

    days_of_results = 90
    if runs_per_week * days_of_results > 2000:
        # testgrid has a limit on number of test runs to show for a job
        days_of_results = math.floor(2000 / runs_per_week)
    annotations = {
        'testgrid-dashboards': ', '.join(sorted(dashboards)),
        'testgrid-days-of-results': str(days_of_results),
        'testgrid-tab-name': tab,
    }
    for (k, v) in spec.items():
        annotations[f"test.kops.k8s.io/{k}"] = v or ""

    extra = yaml.dump({'annotations': annotations}, width=9999, default_flow_style=False)

    output = f"\n# {jsonspec}\n{job.strip()}\n"
    for line in extra.splitlines():
        output += f"  {line}\n"
    return output, runs_per_week

# Returns a string representing a presubmit prow job YAML
def presubmit_test(branch='master',
                   cloud='aws',
                   distro='u2204',
                   networking='cilium',
                   irsa=True,
                   k8s_version='stable',
                   kops_channel='alpha',
                   name=None,
                   tab_name=None,
                   feature_flags=(),
                   extra_flags=None,
                   extra_dashboards=None,
                   terraform_version=None,
                   test_parallelism=25,
                   test_timeout_minutes=60,
                   skip_regex='',
                   focus_regex=None,
                   run_if_changed=None,
                   optional=False,
                   skip_report=False,
                   always_run=False,
                   scenario=None,
                   env=None,
                   template_path=None):
    # pylint: disable=too-many-statements,too-many-branches,too-many-arguments
    if cloud == 'aws':
        kops_image = distro_images[distro]
        kops_ssh_user = distros_ssh_user[distro]
        kops_ssh_key_path = '/etc/aws-ssh/aws-ssh-private'

    elif cloud == 'gce':
        kops_image = None
        kops_ssh_user = 'prow'
        kops_ssh_key_path = '/etc/ssh-key-secret/ssh-private'

    if extra_flags is None:
        extra_flags = []

    if irsa and cloud == "aws" and scenario is None:
        extra_flags.append("--discovery-store=s3://k8s-kops-prow/discovery")

    marker, k8s_deploy_url, test_package_bucket, test_package_dir = k8s_version_info(k8s_version)
    args = create_args(kops_channel, networking, extra_flags, kops_image)

    # Scenario-specific parameters
    if env is None:
        env = {}

    tmpl_file = "presubmit.yaml.jinja"
    if scenario is not None:
        tmpl_file = "presubmit-scenario.yaml.jinja"
        name_hash = hashlib.md5(name.encode()).hexdigest()
        env['CLOUD_PROVIDER'] = cloud
        env['CLUSTER_NAME'] = f"e2e-{name_hash[0:10]}-{name_hash[11:16]}.test-cncf-aws.k8s.io"
        env['KOPS_STATE_STORE'] = 's3://k8s-kops-prow'
        if extra_flags:
            env['KOPS_EXTRA_FLAGS'] = " ".join(extra_flags)
        if irsa and cloud == "aws":
            env['KOPS_IRSA'] = "true"

    tmpl = jinja2.Environment(loader=loader).get_template(tmpl_file)
    job = tmpl.render(
        job_name=name,
        branch=branch,
        cloud=cloud,
        kops_ssh_key_path=kops_ssh_key_path,
        kops_ssh_user=kops_ssh_user,
        create_args=args,
        k8s_deploy_url=k8s_deploy_url,
        test_parallelism=str(test_parallelism),
        job_timeout=str(test_timeout_minutes + 30) + 'm',
        test_timeout=str(test_timeout_minutes) + 'm',
        marker=marker,
        skip_regex=skip_regex,
        kops_feature_flags=','.join(feature_flags),
        terraform_version=terraform_version,
        test_package_bucket=test_package_bucket,
        test_package_dir=test_package_dir,
        focus_regex=focus_regex,
        run_if_changed=run_if_changed,
        optional='true' if optional else 'false',
        skip_report='true' if skip_report else 'false',
        always_run='true' if always_run else 'false',
        image=image,
        scenario=scenario,
        env=env,
        template_path=template_path,
    )

    spec = {
        'cloud': cloud,
        'networking': networking,
        'distro': distro,
        'k8s_version': k8s_version,
        'kops_channel': kops_channel,
    }
    if feature_flags:
        spec['feature_flags'] = ','.join(feature_flags)
    if extra_flags:
        spec['extra_flags'] = ' '.join(extra_flags)
    jsonspec = json.dumps(spec, sort_keys=True)

    dashboards = [
        'presubmits-kops',
        'kops-presubmits',
        'sig-cluster-lifecycle-kops',
    ]
    if extra_dashboards:
        dashboards.extend(extra_dashboards)

    annotations = {
        'testgrid-dashboards': ', '.join(sorted(dashboards)),
        'testgrid-days-of-results': '90',
        'testgrid-tab-name': tab_name or name,
    }
    for (k, v) in spec.items():
        annotations[f"test.kops.k8s.io/{k}"] = v or ""

    extra = yaml.dump({'annotations': annotations}, width=9999, default_flow_style=False)

    output = f"\n# {jsonspec}{job}\n"
    for line in extra.splitlines():
        output += f"    {line}\n"
    return output

####################
# Grid Definitions #
####################

networking_options = [
    'kubenet',
    'calico',
    'cilium',
    'cilium-etcd',
    'cilium-eni',
    'flannel',
    'kopeio',
]

distro_options = [
    'amzn2',
    'deb10',
    'flatcar',
    'rhel8',
    'u2004',
    'u2204',
]

k8s_versions = [
    "1.23",
    "1.24",
    "1.25",
    "1.26",
]

kops_versions = [
    None, # maps to latest
    "1.25",
    "1.26"
]


############################
# kops-periodics-grid.yaml #
############################
def generate_grid():
    results = []
    # pylint: disable=too-many-nested-blocks
    for networking in networking_options:
        for distro in distro_options:
            for k8s_version in k8s_versions:
                for kops_version in kops_versions:
                    if networking == 'cilium-eni' and kops_version in ['1.25']:
                        continue
                    results.append(
                        build_test(cloud="aws",
                                   distro=distro,
                                   extra_dashboards=['kops-grid'],
                                   k8s_version=k8s_version,
                                   kops_version=kops_version,
                                   networking=networking,
                                   irsa=False)
                    )

    # Manually expand grid coverage for GCP
    # TODO(justinsb): merge into above block when we can
    # pylint: disable=too-many-nested-blocks
    for networking in ['kubenet', 'calico', 'cilium', 'gce']: # TODO: all networking_options:
        for distro in ['u2004']: # TODO: all distro_options:
            for k8s_version in k8s_versions:
                for kops_version in [None]: # TODO: all kops_versions:
                    results.append(
                        build_test(cloud="gce",
                                   runs_per_day=3,
                                   distro=distro,
                                   extra_dashboards=['kops-grid'],
                                   k8s_version=k8s_version,
                                   kops_version=kops_version,
                                   networking=networking,
                                   extra_flags=["--gce-service-account=default"], # Workaround for test-infra#24747 # pylint: disable=line-too-long
                                   )
                    )

    return filter(None, results)

#############################
# kops-periodics-misc2.yaml #
#############################
def generate_misc():
    results = [
        # A one-off scenario testing the k8s.gcr.io mirror
        build_test(name_override="kops-scenario-gcr-mirror",
                   runs_per_day=24,
                   cloud="aws",
                   # Latest runs with a staging AWS CCM, not available in registry.k8s.io
                   k8s_version='1.23',
                   extra_dashboards=['kops-misc']),

        # A one-off scenario testing the artifacts-sandbox.k8s.io mirror
        build_test(name_override="kops-artifacts-sandbox",
                   runs_per_day=24,
                   cloud="aws",
                   k8s_version='stable',
                   extra_dashboards=['kops-misc'],
                   scenario='smoketest',
                   env={
                       'KOPS_BASE_URL': "https://artifacts-sandbox.k8s.io/binaries/kops/1.26.0-beta.2/", # pylint: disable=line-too-long
                       'KOPS_VERSION': "v1.26.0-beta.2",
                       'K8S_VERSION': "v1.25.0",
                       'KOPS_SKIP_E2E': '1',
                       'KOPS_CONTROL_PLANE_SIZE': '3',
                   }),

        # A one-off scenario testing arm64
        build_test(name_override="kops-scenario-arm64",
                   cloud="aws",
                   distro="u2204arm64",
                   runs_per_day=1,
                   extra_flags=["--zones=eu-central-1a",
                                "--node-size=m6g.large",
                                "--master-size=m6g.large"],
                   extra_dashboards=['kops-misc']),

        # A special test for Calico CNI on Debian 11
        build_test(name_override="kops-aws-cni-calico-deb11",
                   cloud="aws",
                   distro="deb11",
                   networking="calico",
                   runs_per_day=3,
                   extra_dashboards=['kops-network-plugins']),
        # A special test for Calico CNI on Flatcar
        build_test(name_override="kops-aws-cni-calico-flatcar",
                   cloud="aws",
                   distro="flatcararm64",
                   networking="calico",
                   runs_per_day=3,
                   extra_dashboards=['kops-distros', 'kops-network-plugins']),

        # A special test for IPv6 using Calico CNI
        build_test(name_override="kops-aws-cni-calico-ipv6",
                   cloud="aws",
                   distro="u2204arm64",
                   networking="calico",
                   runs_per_day=3,
                   extra_flags=['--ipv6',
                                '--topology=private',
                                '--bastion',
                                '--zones=us-west-2a',
                                ],
                   extra_dashboards=['kops-network-plugins', 'kops-ipv6']),
        # A special test for IPv6 using Cilium CNI
        build_test(name_override="kops-aws-cni-cilium-ipv6",
                   cloud="aws",
                   distro="u2204arm64",
                   networking="cilium",
                   runs_per_day=3,
                   extra_flags=['--ipv6',
                                '--topology=private',
                                '--bastion',
                                '--zones=us-west-2a',
                                ],
                   extra_dashboards=['kops-network-plugins', 'kops-ipv6']),
        # A special test for IPv6 on Flatcar
        build_test(name_override="kops-aws-ipv6-flatcar",
                   cloud="aws",
                   distro="flatcararm64",
                   runs_per_day=3,
                   extra_flags=['--ipv6',
                                '--topology=private',
                                '--bastion',
                                ],
                   extra_dashboards=['kops-distros', 'kops-ipv6']),
        # A special test for IPv6 using Calico on Flatcar
        build_test(name_override="kops-aws-cni-calico-ipv6-flatcar",
                   cloud="aws",
                   distro="flatcararm64",
                   networking="calico",
                   runs_per_day=3,
                   extra_flags=['--ipv6',
                                '--topology=private',
                                '--bastion',
                                ],
                   extra_dashboards=['kops-distros', 'kops-network-plugins', 'kops-ipv6']),

        # A special test for disabling IRSA
        build_test(name_override="kops-scenario-no-irsa",
                   cloud="aws",
                   distro="u2204arm64",
                   runs_per_day=3,
                   irsa=False,
                   extra_flags=['--api-loadbalancer-type=public',
                                ],
                   extra_dashboards=['kops-misc']),

        # A special test for warm pool
        build_test(name_override="kops-warm-pool",
                   distro="u2204arm64",
                   runs_per_day=3,
                   networking="cilium",
                   extra_flags=['--api-loadbalancer-type=public',
                                '--set=cluster.spec.cloudProvider.aws.warmPool.minSize=1'
                                ],
                   extra_dashboards=['kops-misc']),

        # A special test for private topology
        build_test(name_override="kops-aws-private",
                   cloud="aws",
                   distro="u2204arm64",
                   runs_per_day=3,
                   networking="calico",
                   extra_flags=['--topology=private',
                                '--bastion',
                                ],
                   extra_dashboards=['kops-misc']),

        build_test(name_override="kops-scenario-terraform",
                   distro="u2204arm64",
                   runs_per_day=1,
                   terraform_version="1.0.5",
                   extra_flags=[
                       "--zones=us-west-1a",
                   ],
                   extra_dashboards=['kops-misc']),
        build_test(name_override="kops-scenario-ipv6-terraform",
                   distro="u2204arm64",
                   runs_per_day=1,
                   terraform_version="1.0.5",
                   extra_flags=[
                       '--ipv6',
                       '--topology=private',
                       '--bastion',
                       "--zones=us-west-1a",
                   ],
                   extra_dashboards=['kops-misc', 'kops-ipv6']),

        build_test(name_override="kops-aws-ha-euwest1",
                   distro="u2204arm64",
                   networking="calico",
                   kops_channel="alpha",
                   runs_per_day=3,
                   extra_flags=[
                       "--master-count=3",
                       "--zones=eu-west-1a,eu-west-1b,eu-west-1c"
                   ],
                   extra_dashboards=["kops-misc"]),

        build_test(name_override="kops-aws-arm64-release",
                   k8s_version="latest",
                   distro="u2204arm64",
                   networking="calico",
                   kops_channel="alpha",
                   runs_per_day=3,
                   extra_flags=["--zones=eu-central-1a",
                                "--node-size=m6g.large",
                                "--master-size=m6g.large"],
                   extra_dashboards=["kops-misc"]),

        build_test(name_override="kops-aws-arm64-ci",
                   k8s_version="ci",
                   distro="u2204arm64",
                   networking="calico",
                   kops_channel="alpha",
                   runs_per_day=3,
                   extra_flags=["--zones=eu-central-1a",
                                "--node-size=m6g.large",
                                "--master-size=m6g.large"],
                   extra_dashboards=["kops-misc"]),

        build_test(name_override="kops-aws-arm64-conformance",
                   k8s_version="ci",
                   distro="u2204arm64",
                   networking="calico",
                   kops_channel="alpha",
                   runs_per_day=3,
                   extra_flags=["--zones=eu-central-1a",
                                "--node-size=m6g.large",
                                "--master-size=m6g.large"],
                   skip_regex=r'\[Slow\]|\[Serial\]|\[Flaky\]',
                   focus_regex=r'\[Conformance\]|\[NodeConformance\]',
                   extra_dashboards=["kops-misc"]),

        build_test(name_override="kops-aws-amd64-conformance",
                   k8s_version="ci",
                   distro='u2204',
                   networking="calico",
                   kops_channel="alpha",
                   runs_per_day=3,
                   extra_flags=["--node-size=c5.large",
                                "--master-size=c5.large"],
                   skip_regex=r'\[Slow\]|\[Serial\]|\[Flaky\]',
                   focus_regex=r'\[Conformance\]|\[NodeConformance\]',
                   extra_dashboards=["kops-misc"]),

        build_test(name_override="kops-aws-updown",
                   k8s_version="stable",
                   distro="u2204arm64",
                   networking="calico",
                   kops_channel="alpha",
                   kops_version="https://storage.googleapis.com/kops-ci/bin/latest-ci.txt",
                   publish_version_marker="gs://kops-ci/bin/latest-ci-updown-green.txt",
                   runs_per_day=24,
                   focus_regex=r'\[k8s.io\]\sNetworking.*\[Conformance\]',
                   extra_dashboards=["kops-misc"]),

        build_test(name_override="kops-aws-aws-load-balancer-controller",
                   cloud="aws",
                   networking="cilium",
                   kops_channel="alpha",
                   k8s_version="stable",
                   runs_per_day=3,
                   scenario="aws-lb-controller",
                   extra_dashboards=['kops-misc']),

        build_test(name_override="kops-aws-keypair-rotation-ha",
                   cloud="aws",
                   kops_channel="alpha",
                   k8s_version="stable",
                   runs_per_day=1,
                   test_timeout_minutes=240,
                   scenario="keypair-rotation",
                   env={'KOPS_CONTROL_PLANE_SIZE': '3'},
                   extra_dashboards=['kops-misc']),

        build_test(name_override="kops-aws-metrics-server",
                   cloud="aws",
                   networking="cilium",
                   kops_channel="alpha",
                   k8s_version="stable",
                   runs_per_day=3,
                   scenario="metrics-server",
                   extra_dashboards=['kops-misc']),

        build_test(name_override="kops-aws-pod-identity-webhook",
                   cloud="aws",
                   networking="cilium",
                   kops_channel="alpha",
                   k8s_version="stable",
                   runs_per_day=3,
                   scenario="podidentitywebhook",
                   extra_dashboards=['kops-misc']),

        build_test(name_override="kops-aws-addon-resource-tracking",
                   cloud="aws",
                   networking="cilium",
                   kops_channel="alpha",
                   k8s_version="stable",
                   runs_per_day=3,
                   scenario="addon-resource-tracking",
                   extra_dashboards=['kops-misc']),

        build_test(name_override="kops-aws-external-dns",
                   cloud="aws",
                   distro="u2204arm64",
                   networking="cilium",
                   kops_channel="alpha",
                   runs_per_day=3,
                   extra_flags=[
                       "--set=cluster.spec.externalDNS.provider=external-dns",
                   ],
                   extra_dashboards=['kops-misc']),

        build_test(name_override="kops-aws-apiserver-nodes",
                   cloud="aws",
                   distro="u2204arm64",
                   runs_per_day=3,
                   template_path="/home/prow/go/src/k8s.io/kops/tests/e2e/templates/apiserver.yaml.tmpl", # pylint: disable=line-too-long
                   extra_dashboards=['kops-misc'],
                   feature_flags=['APIServerNodes']),

        build_test(name_override="kops-aws-karpenter",
                   distro="u2204arm64",
                   networking="cilium",
                   kops_channel="alpha",
                   runs_per_day=1,
                   extra_flags=[
                       "--instance-manager=karpenter",
                       "--master-size=c6g.xlarge",
                   ],
                   feature_flags=['Karpenter'],
                   extra_dashboards=["kops-misc"],
                   skip_regex=r'\[Slow\]|\[Serial\]|\[Disruptive\]|\[Flaky\]|\[Feature:.+\]|nfs|NFS|Gluster|Services.*rejected.*endpoints|TCP.CLOSE_WAIT|external.IP.is.not.assigned.to.a.node|same.port.number.but.different.protocols|same.hostPort.but.different.hostIP.and.protocol|should.create.a.Pod.with.SCTP.HostPort|Services.should.create.endpoints.for.unready.pods|Services.should.be.able.to.connect.to.terminating.and.unready.endpoints.if.PublishNotReadyAddresses.is.true|should.verify.that.all.nodes.have.volume.limits|In-tree.Volumes|LoadBalancers.should.be.able.to.preserve.UDP.traffic'), # pylint: disable=line-too-long

        build_test(name_override="kops-aws-ipv6-karpenter",
                   distro="u2204arm64",
                   networking="cilium",
                   kops_channel="alpha",
                   runs_per_day=1,
                   extra_flags=[
                       "--instance-manager=karpenter",
                       '--ipv6',
                       '--topology=private',
                       '--bastion',
                       "--master-size=c6g.xlarge",
                   ],
                   feature_flags=['Karpenter'],
                   extra_dashboards=["kops-misc", "kops-ipv6"],
                   skip_regex=r'\[Slow\]|\[Serial\]|\[Disruptive\]|\[Flaky\]|\[Feature:.+\]|nfs|NFS|Gluster|Services.*rejected.*endpoints|TCP.CLOSE_WAIT|external.IP.is.not.assigned.to.a.node|same.port.number.but.different.protocols|same.hostPort.but.different.hostIP.and.protocol|should.create.a.Pod.with.SCTP.HostPort|Services.should.create.endpoints.for.unready.pods|Services.should.be.able.to.connect.to.terminating.and.unready.endpoints.if.PublishNotReadyAddresses.is.true|should.verify.that.all.nodes.have.volume.limits|In-tree.Volumes|LoadBalancers.should.be.able.to.preserve.UDP.traffic'), # pylint: disable=line-too-long

        # [sig-storage, @jsafrane] A one-off scenario testing SELinux features, because kops
        # is the only way how to get Kubernetes on a Linux with SELinux in enforcing mode in CI.
        # Test the latest kops and CI build of Kubernetes (=almost master).
        build_test(name_override="kops-aws-selinux",
                   # RHEL8 VM image is enforcing SELinux by default.
                   distro="rhel8",
                   networking="cilium",
                   k8s_version="ci",
                   kops_channel="alpha",
                   feature_flags=['SELinuxMount'],
                   extra_flags=["--kubernetes-feature-gates=SELinuxMountReadWriteOncePod,ReadWriteOncePod"], # pylint: disable=line-too-long
                   focus_regex=r"\[Feature:SELinux\]",
                   # Skip:
                   # - Feature:Volumes: skips iSCSI and Ceph tests, they don't have client tools
                   #   installed on nodes.
                   # - Driver: nfs: NFS does not have client tools installed on nodes.
                   # - Driver: local: this is optimization only, the volume plugin does not
                   #   support SELinux and there are several subvariants of local volumes
                   #   that multiply nr. of tests.
                   skip_regex=r"\[Feature:Volumes\]|\[Driver:.nfs\]|\[Driver:.local\]",
                   # [Serial] and [Disruptive] are intentionally not skipped, therefore run
                   # everything as serial.
                   test_parallelism=1,
                   # Serial and Disruptive tests can be slow.
                   test_timeout_minutes=120,
                   runs_per_day=3),
    ]
    return results

################################
# kops-periodics-versions.yaml #
################################
def generate_conformance():
    results = []
    for version in ['1.26', '1.25']:
        results.append(
            build_test(
                k8s_version=version,
                kops_version=version,
                kops_channel='alpha',
                name_override=f"kops-aws-conformance-{version.replace('.', '-')}",
                networking='calico',
                test_parallelism=1,
                test_timeout_minutes=150,
                extra_dashboards=['kops-conformance'],
                runs_per_day=1,
                focus_regex=r'\[Conformance\]',
                skip_regex=r'\[NoSkip\]',
            )
        )
        results.append(
            build_test(
                k8s_version=version,
                kops_version=version,
                kops_channel='alpha',
                name_override=f"kops-aws-conformance-arm64-{version.replace('.', '-')}",
                networking='calico',
                distro="u2204arm64",
                extra_flags=["--zones=eu-central-1a",
                             "--node-size=t4g.large",
                             "--master-size=t4g.large"],
                test_parallelism=1,
                test_timeout_minutes=150,
                extra_dashboards=['kops-conformance'],
                runs_per_day=1,
                focus_regex=r'\[Conformance\]',
                skip_regex=r'\[NoSkip\]',
            )
        )
    return results

###############################
# kops-periodics-distros.yaml #
###############################
distros = ['debian10', 'debian11',
           'ubuntu1804', 'ubuntu2004', 'ubuntu2004arm64', 'ubuntu2204', 'ubuntu2204arm64',
           'amazonlinux2', 'al2023', 'rhel8', 'rocky8',
           'flatcar']
def generate_distros():
    results = []
    for distro in distros:
        distro_short = distro.replace('ubuntu', 'u').replace('debian', 'deb').replace('amazonlinux', 'amzn') # pylint: disable=line-too-long
        extra_flags = []
        if 'arm64' in distro:
            extra_flags = [
                "--zones=eu-west-1a",
                "--node-size=m6g.large",
                "--master-size=m6g.large"
            ]
        results.append(
            build_test(distro=distro_short,
                       networking='cilium',
                       k8s_version='stable',
                       kops_channel='alpha',
                       name_override=f"kops-aws-distro-{distro}",
                       extra_dashboards=['kops-distros'],
                       extra_flags=extra_flags,
                       runs_per_day=3,
                       )
        )
    return results

###############################
# kops-presubmits-distros.yaml #
###############################
def generate_presubmits_distros():
    results = []
    for distro in distros:
        distro_short = distro.replace('ubuntu', 'u').replace('debian', 'deb').replace('amazonlinux', 'amzn') # pylint: disable=line-too-long
        extra_flags = []
        if 'arm64' in distro:
            extra_flags = [
                "--zones=eu-west-1a",
                "--node-size=m6g.large",
                "--master-size=m6g.large"
            ]
        results.append(
            presubmit_test(
                distro=distro_short,
                networking='calico',
                k8s_version='stable',
                kops_channel='alpha',
                name=f"pull-kops-aws-distro-{distro}",
                tab_name=f"e2e-{distro}",
                extra_flags=extra_flags,
                always_run=False,
            )
        )
    return results

#######################################
# kops-periodics-network-plugins.yaml #
#######################################
def generate_network_plugins():

    plugins = ['amazon-vpc', 'calico', 'canal', 'cilium', 'cilium-etcd', 'cilium-eni', 'flannel', 'kopeio', 'kuberouter', 'weave'] # pylint: disable=line-too-long
    results = []
    for plugin in plugins:
        networking_arg = plugin.replace('amazon-vpc', 'amazonvpc').replace('kuberouter', 'kube-router') # pylint: disable=line-too-long
        k8s_version = 'stable'
        if plugin == 'weave':
            k8s_version = '1.22'
        distro = 'u2204'
        if plugin == 'amazon-vpc':
            distro = 'u2004'
        results.append(
            build_test(
                distro=distro,
                k8s_version=k8s_version,
                kops_channel='alpha',
                name_override=f"kops-aws-cni-{plugin}",
                networking=networking_arg,
                extra_flags=['--node-size=t3.large'],
                extra_dashboards=['kops-network-plugins'],
                runs_per_day=3,
            )
        )
    return results

################################
# kops-periodics-upgrades.yaml #
################################
def generate_upgrades():

    kops24 = 'v1.24.4'
    kops25 = 'v1.25.2'
    kops26 = 'v1.26.0-beta.1'

    versions_list = [
        #  kops    k8s          kops      k8s
        # 1.25 release branch
        ((kops24, 'v1.19.16'), ('1.25', 'v1.20.6')),
        ((kops24, 'v1.23.1'), ('1.25', 'v1.23.1')),
        ((kops25, 'v1.21.14'), ('1.25', 'v1.22.13')),
        ((kops25, 'v1.25.0'), ('1.25', 'v1.25.0')),
        # 1.26 release branch
        ((kops24, 'v1.20.6'), ('1.26', 'v1.21.7')),
        ((kops24, 'v1.24.0'), ('1.26', 'v1.25.0')),
        ((kops25, 'v1.20.6'), ('1.26', 'v1.21.7')),
        ((kops25, 'v1.25.0'), ('1.26', 'v1.26.0')),
        ((kops26, 'v1.21.14'), ('1.26', 'v1.22.13')),
        ((kops26, 'v1.26.0'), ('1.26', 'v1.26.0')),
        # 1.24 upgrade to latest
        ((kops24, 'v1.23.1'), ('latest', 'v1.24.0')),
        ((kops24, 'v1.24.0'), ('latest', 'v1.25.0')),
        # 1.25 upgrade to latest
        ((kops25, 'v1.24.0'), ('latest', 'v1.25.0')),
        ((kops25, 'v1.25.0'), ('latest', 'v1.26.0')),
        # 1.26 upgrade to latest
        ((kops26, 'v1.21.14'), ('latest', 'v1.22.1')),
        ((kops26, 'v1.24.0'), ('latest', 'v1.25.0')),
        ((kops26, 'v1.25.0'), ('latest', 'v1.25.0')),
        ((kops26, 'v1.25.0'), ('latest', 'v1.26.0')),
        # we should have an upgrade test for every supported K8s version
        (('latest', 'v1.26.0'), ('latest', 'latest')),
        (('latest', 'v1.25.0'), ('latest', 'v1.26.0')),
        (('latest', 'v1.24.0'), ('latest', 'v1.25.0')),
        (('latest', 'v1.23.0'), ('latest', 'v1.24.0')),
        (('latest', 'v1.22.4'), ('latest', 'v1.23.0')),
        # kOps latest should always be able to upgrade from stable to latest and stable to ci
        (('latest', 'stable'), ('latest', 'latest')),
        (('latest', 'stable'), ('latest', 'ci')),
    ]
    def shorten(version):
        version = re.sub(r'^v', '', version)
        version = re.sub(r'^(\d+\.\d+)\.\d+$', r'\g<1>', version)
        return version.replace('.', '')
    results = []
    for versions in versions_list:
        kops_a = versions[0][0]
        k8s_a = versions[0][1]
        kops_b = versions[1][0]
        k8s_b = versions[1][1]
        job_name = f"kops-aws-upgrade-k{shorten(k8s_a)}-ko{shorten(kops_a)}-to-k{shorten(k8s_b)}-ko{shorten(kops_b)}" # pylint: disable=line-too-long
        env = {
            'KOPS_VERSION_A': kops_a,
            'K8S_VERSION_A': k8s_a,
            'KOPS_VERSION_B': kops_b,
            'K8S_VERSION_B': k8s_b,
        }
        addonsenv = {
            'KOPS_VERSION_A': kops_a,
            'K8S_VERSION_A': k8s_a,
            'KOPS_VERSION_B': kops_b,
            'K8S_VERSION_B': k8s_b,
            'KOPS_SKIP_E2E': '1',
            'KOPS_TEMPLATE': 'tests/e2e/templates/many-addons.yaml.tmpl',
            'KOPS_CONTROL_PLANE_SIZE': '3',
        }
        results.append(
            build_test(name_override=job_name,
                       distro='u2004',
                       networking='calico',
                       irsa=k8s_a >= 'v1.22',
                       k8s_version='stable',
                       kops_channel='alpha',
                       extra_dashboards=['kops-upgrades'],
                       runs_per_day=8,
                       test_timeout_minutes=120,
                       scenario='upgrade-ab',
                       env=env,
                       )
        )
        # k8s 1.19 has issues with our server side apply logic for addons
        if 'v1.19.' in k8s_a:
            continue
        results.append(
            build_test(name_override=job_name + "-many-addons",
                       distro='u2004',
                       networking='calico',
                       irsa=k8s_a >= 'v1.22',
                       k8s_version='stable',
                       kops_channel='alpha',
                       extra_dashboards=['kops-upgrades-many-addons'],
                       test_timeout_minutes=120,
                       runs_per_day=4,
                       scenario='upgrade-ab',
                       env=addonsenv,
                       )
        )
    return results

################################
# kops-periodics-scale.yaml #
################################
def generate_scale():
    results = [
        build_test(
            name_override='kops-aws-scale',
            extra_dashboards=[],
            runs_per_day=1,
            scenario='scalability',
        )
    ]
    return results

###############################
# kops-presubmits-scale.yaml #
###############################
def generate_presubmits_scale():
    results = [
        presubmit_test(
            name='presubmit-kops-aws-scale',
            scenario='scalability',
            always_run=False,
        )
    ]
    return results

################################
# kops-periodics-versions.yaml #
################################
def generate_versions():
    results = [
        build_test(
            k8s_version='ci',
            kops_channel='alpha',
            name_override='kops-aws-k8s-latest',
            networking='calico',
            extra_dashboards=['kops-versions'],
            runs_per_day=8,
            # This version marker is only used by the k/k presubmit job
            publish_version_marker='gs://kops-ci/bin/latest-ci-green.txt',
        )
    ]
    for version in ['1.26', '1.25', '1.24', '1.23', '1.22']:
        results.append(
            build_test(
                k8s_version=version,
                irsa=version >= '1.22',
                kops_channel='alpha',
                name_override=f"kops-aws-k8s-{version.replace('.', '-')}",
                networking='calico',
                extra_dashboards=['kops-versions'],
                runs_per_day=8,
            )
        )
    return results

######################
# kops-pipeline.yaml #
######################
def generate_pipeline():
    results = []
    for version in ['master', '1.26', '1.25']:
        branch = version if version == 'master' else f"release-{version}"
        publish_version_marker = f"gs://kops-ci/markers/{branch}/latest-ci-updown-green.txt"
        kops_version = f"https://storage.googleapis.com/k8s-staging-kops/kops/releases/markers/{branch}/latest-ci.txt" # pylint: disable=line-too-long
        results.append(
            build_test(
                k8s_version=version.replace('master', 'latest'),
                kops_version=kops_version,
                kops_channel='alpha',
                name_override=f"kops-pipeline-updown-kops{version.replace('.', '')}",
                networking='calico',
                extra_dashboards=['kops-versions'],
                runs_per_day=24,
                skip_regex=r'\[Slow\]|\[Serial\]',
                focus_regex=r'\[k8s.io\]\sNetworking.*\[Conformance\]',
                publish_version_marker=publish_version_marker,
            )
        )
    return results

########################################
# kops-presubmits-network-plugins.yaml #
########################################
def generate_presubmits_network_plugins():
    plugins = {
        'amazonvpc': r'^(upup\/models\/cloudup\/resources\/addons\/networking\.amazon-vpc-routed-eni\/|pkg\/model\/(firewall|components\/containerd|components\/kubeproxy|iam\/iam_builder)\.go|nodeup\/pkg\/model\/kubelet\.go)', # pylint: disable=line-too-long
        'calico': r'^(upup\/models\/cloudup\/resources\/addons\/networking\.projectcalico\.org\/|pkg\/model\/(components\/containerd|firewall|pki|iam\/iam_builder)\.go|nodeup\/pkg\/model\/networking\/calico\.go)', # pylint: disable=line-too-long
        'canal': r'^(upup\/models\/cloudup\/resources\/addons\/networking\.projectcalico\.org\.canal\/)', # pylint: disable=line-too-long
        'cilium': r'^(upup\/models\/cloudup\/resources\/addons\/networking\.cilium\.io\/|pkg\/model\/(components\/containerd|firewall|components\/cilium|iam\/iam_builder)\.go|nodeup\/pkg\/model\/(context|networking\/cilium)\.go)', # pylint: disable=line-too-long
        'cilium-etcd': r'^(upup\/models\/cloudup\/resources\/addons\/networking\.cilium\.io\/|pkg\/model\/(components\/containerd|firewall|components\/cilium|iam\/iam_builder)\.go|nodeup\/pkg\/model\/(context|networking\/cilium)\.go)', # pylint: disable=line-too-long
        'cilium-eni': r'^(upup\/models\/cloudup\/resources\/addons\/networking\.cilium\.io\/|pkg\/model\/(components\/containerd|firewall|components\/cilium|iam\/iam_builder)\.go|nodeup\/pkg\/model\/(context|networking\/cilium)\.go)', # pylint: disable=line-too-long
        'flannel': r'^(upup\/models\/cloudup\/resources\/addons\/networking\.flannel\/|pkg\/model\/components\/containerd\.go)', # pylint: disable=line-too-long
        'kuberouter': r'^(upup\/models\/cloudup\/resources\/addons\/networking\.kuberouter\/|pkg\/model\/components\/containerd\.go)', # pylint: disable=line-too-long
        'weave': r'^(upup\/models\/cloudup\/resources\/addons\/networking\.weave\/)' # pylint: disable=line-too-long
    }
    supports_ipv6 = {'amazonvpc', 'calico', 'cilium'}
    results = []
    for plugin, run_if_changed in plugins.items():
        k8s_version = 'stable'
        networking_arg = plugin
        optional = False
        distro = 'u2204arm64'
        if plugin == 'amazonvpc':
            distro = 'u2004'
            optional = True
        if plugin == 'kuberouter':
            networking_arg = 'kube-router'
        if plugin == 'weave':
            distro = 'u2204'
            k8s_version = '1.22'
        extra_flags = ['--node-size=t3.large']
        if 'arm64' in distro:
            extra_flags = ["--node-size=t4g.large"]
        results.append(
            presubmit_test(
                distro=distro,
                k8s_version=k8s_version,
                kops_channel='alpha',
                name=f"pull-kops-e2e-cni-{plugin}",
                tab_name=f"e2e-{plugin}",
                networking=networking_arg,
                extra_flags=extra_flags,
                run_if_changed=run_if_changed,
                optional=optional,
            )
        )
        if plugin in supports_ipv6:
            if plugin == 'amazonvpc':
                run_if_changed = None
            results.append(
                presubmit_test(
                    name=f"pull-kops-e2e-cni-{plugin}-ipv6",
                    distro=distro,
                    tab_name=f"e2e-{plugin}-ipv6",
                    networking=networking_arg,
                    extra_flags=['--ipv6',
                                 '--topology=private',
                                 '--bastion',
                                 '--zones=us-west-2a',
                                 ],
                    run_if_changed=run_if_changed,
                    optional=optional,
                )
            )
    return results

############################
# kops-presubmits-e2e.yaml #
############################
def generate_presubmits_e2e():
    jobs = [
        presubmit_test(
            distro='u2204arm64',
            k8s_version='ci',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-ci',
            networking='calico',
            tab_name='e2e-containerd-ci',
            always_run=False,
            focus_regex=r'\[Conformance\]|\[NodeConformance\]',
        ),
        presubmit_test(
            distro='u2204arm64',
            k8s_version='ci',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-ci-ha',
            networking='calico',
            extra_flags=[
                "--master-count=3",
                "--node-count=6",
                "--zones=eu-central-1a,eu-central-1b,eu-central-1c"],
            tab_name='e2e-containerd-ci-ha',
            always_run=False,
            focus_regex=r'\[Conformance\]|\[NodeConformance\]',
        ),
        presubmit_test(
            distro='u2204arm64',
            k8s_version='stable',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-aws-calico',
            networking='calico',
            tab_name='e2e-aws-calico',
            always_run=True,
        ),
        presubmit_test(
            cloud='gce',
            k8s_version='stable',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-gce-cilium',
            networking='cilium',
            tab_name='e2e-gce-cilium',
            always_run=True,
            extra_flags=["--gce-service-account=default"], # Workaround for test-infra#24747
        ),
        presubmit_test(
            cloud='gce',
            k8s_version='stable',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-gce-cilium-etcd',
            networking='cilium-etcd',
            tab_name='e2e-gce-cilium-etcd',
            always_run=False,
            extra_flags=["--gce-service-account=default"], # Workaround for test-infra#24747
        ),
        presubmit_test(
            cloud='gce',
            k8s_version='stable',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-gce-ipalias',
            networking='gce',
            tab_name='e2e-gce',
            always_run=False,
            extra_flags=["--gce-service-account=default"], # Workaround for test-infra#24747
        ),
        presubmit_test(
            cloud='gce',
            k8s_version='stable',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-gce-long-cluster-name',
            networking='cilium',
            tab_name='e2e-gce-long-name',
            always_run=False,
            extra_flags=["--gce-service-account=default"], # Workaround for test-infra#24747
        ),
        presubmit_test(
            cloud='gce',
            k8s_version='ci',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-gce-ci',
            networking='cilium',
            tab_name='e2e-gce-ci',
            always_run=False,
            extra_flags=["--gce-service-account=default"], # Workaround for test-infra#24747
        ),
        presubmit_test(
            cloud='gce',
            k8s_version='stable',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-gce-calico-u2004-k22-containerd',
            networking='calico',
            tab_name='pull-kops-e2e-k8s-gce-calico-u2004-k22-containerd',
            always_run=False,
            feature_flags=['GoogleCloudBucketACL'],
            extra_flags=["--gce-service-account=default"], # Workaround for test-infra#24747
        ),
        # A special test for AWS Cloud-Controller-Manager
        presubmit_test(
            name="pull-kops-e2e-aws-cloud-controller-manager",
            cloud="aws",
            distro="u2004",
            k8s_version="ci",
            extra_flags=['--set=cluster.spec.cloudControllerManager.cloudProvider=aws'],
            tab_name='e2e-ccm',
        ),

        presubmit_test(
            name="pull-kops-e2e-aws-load-balancer-controller",
            cloud="aws",
            distro="u2004",
            networking="calico",
            scenario="aws-lb-controller",
            tab_name="pull-kops-e2e-aws-load-balancer-controller",
        ),

        presubmit_test(
            name="pull-kops-e2e-addon-resource-tracking",
            cloud="aws",
            distro="u2004",
            networking="calico",
            scenario="addon-resource-tracking",
            tab_name="pull-kops-e2e-aws-addon-resource-tracking",
        ),

        presubmit_test(
            name="pull-kops-e2e-metrics-server",
            cloud="aws",
            distro="u2004",
            networking="calico",
            scenario="metrics-server",
            tab_name="pull-kops-e2e-aws-metrics-server",
        ),

        presubmit_test(
            name="pull-kops-e2e-pod-identity-webhook",
            cloud="aws",
            distro="u2004",
            networking="calico",
            scenario="podidentitywebhook",
            tab_name="pull-kops-e2e-aws-pod-identity-webhook",
        ),

        presubmit_test(
            name="pull-kops-e2e-aws-external-dns",
            cloud="aws",
            networking="calico",
            extra_flags=[
                '--set=cluster.spec.externalDNS.provider=external-dns'
            ],
        ),
        presubmit_test(
            name="pull-kops-e2e-aws-node-local-dns",
            cloud="aws",
            distro='u2204arm64',
            extra_flags=[
                '--set=cluster.spec.kubeDNS.nodeLocalDNS.enabled=true'
            ],
        ),

        presubmit_test(
            name="pull-kops-e2e-aws-apiserver-nodes",
            cloud="aws",
            template_path="/home/prow/go/src/k8s.io/kops/tests/e2e/templates/apiserver.yaml.tmpl",
            feature_flags=['APIServerNodes']
        ),

        presubmit_test(
            name="pull-kops-e2e-arm64",
            cloud="aws",
            distro="u2204arm64",
            networking="calico",
            extra_flags=["--zones=eu-central-1a",
                         "--node-size=m6g.large",
                         "--master-size=m6g.large"],
        ),

        presubmit_test(
            name="pull-kops-e2e-aws-dns-none",
            cloud="aws",
            distro="u2204arm64",
            networking="calico",
            extra_flags=["--dns=none"],
        ),
        presubmit_test(
            name="pull-kops-e2e-gce-dns-none",
            cloud="gce",
            networking="calico",
            extra_flags=["--dns=none", "--gce-service-account=default"],
        ),

        presubmit_test(
            name="pull-kops-e2e-aws-nlb",
            cloud="aws",
            distro="u2204arm64",
            networking="calico",
            extra_flags=[
                "--api-loadbalancer-type=public",
                "--api-loadbalancer-class=network"
            ],
        ),

        presubmit_test(
            name="pull-kops-e2e-aws-terraform",
            cloud="aws",
            distro="u2204arm64",
            terraform_version="1.0.5",
        ),
        presubmit_test(
            name="pull-kops-e2e-aws-ipv6-terraform",
            cloud="aws",
            distro="u2204arm64",
            terraform_version="1.0.5",
            extra_flags=[
                '--ipv6',
                '--bastion',
            ],
        ),

        presubmit_test(
            branch='master',
            k8s_version='1.24',
            kops_channel='alpha',
            name='pull-kops-latest-e2e-aws-k8s-1-24',
            networking='calico',
            tab_name='e2e-aws-1-24',
            always_run=False,
        ),
        presubmit_test(
            branch='master',
            k8s_version='1.23',
            kops_channel='alpha',
            name='pull-kops-latest-e2e-aws-k8s-1-23',
            networking='calico',
            extra_flags=['--set=cluster.spec.cloudControllerManager.cloudProvider=aws'],
            tab_name='e2e-aws-1-23',
            always_run=False,
        ),
        presubmit_test(
            branch='master',
            k8s_version='1.22',
            kops_channel='alpha',
            name='pull-kops-latest-e2e-aws-k8s-1-22',
            networking='calico',
            extra_flags=['--set=cluster.spec.cloudControllerManager.cloudProvider=aws'],
            tab_name='e2e-aws-1-22',
            always_run=False,
        ),

        presubmit_test(
            branch='release-1.26',
            k8s_version='1.26',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-aws-calico-1-26',
            networking='calico',
            tab_name='e2e-1-26',
            always_run=True,
        ),
        presubmit_test(
            branch='release-1.25',
            k8s_version='1.25',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-aws-calico-1-25',
            networking='calico',
            tab_name='e2e-1-25',
            always_run=True,
        ),
        presubmit_test(
            branch='release-1.24',
            k8s_version='1.24',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-aws-calico-1-24',
            networking='calico',
            tab_name='e2e-1-24',
            always_run=True,
        ),
        presubmit_test(
            branch='release-1.23',
            k8s_version='1.23',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-aws-calico-1-23',
            networking='calico',
            tab_name='e2e-1-23',
            always_run=True,
        ),
        presubmit_test(
            branch='release-1.22',
            k8s_version='1.22',
            kops_channel='alpha',
            name='pull-kops-e2e-k8s-aws-calico-1-22',
            networking='calico',
            tab_name='e2e-1-22',
            always_run=True,
        ),
        presubmit_test(
            distro='u2204arm64',
            name="pull-kops-e2e-aws-karpenter",
            run_if_changed=r'^upup\/models\/cloudup\/resources\/addons\/karpenter\.sh\/',
            networking="cilium",
            kops_channel="alpha",
            extra_flags=[
                "--instance-manager=karpenter",
                "--master-size=c6g.xlarge",
            ],
            feature_flags=['Karpenter'],
            skip_regex=r'\[Slow\]|\[Serial\]|\[Disruptive\]|\[Flaky\]|\[Feature:.+\]|nfs|NFS|Gluster|Services.*rejected.*endpoints|TCP.CLOSE_WAIT|external.IP.is.not.assigned.to.a.node|same.port.number.but.different.protocols|same.hostPort.but.different.hostIP.and.protocol|should.create.a.Pod.with.SCTP.HostPort|Services.should.create.endpoints.for.unready.pods|Services.should.be.able.to.connect.to.terminating.and.unready.endpoints.if.PublishNotReadyAddresses.is.true|should.verify.that.all.nodes.have.volume.limits|In-tree.Volumes|LoadBalancers.should.be.able.to.preserve.UDP.traffic' # pylint: disable=line-too-long
        ),
        presubmit_test(
            name="pull-kops-e2e-aws-upgrade-k124-ko124-to-k125-kolatest",
            optional=True,
            distro='u2004',
            networking='cilium',
            k8s_version='stable',
            kops_channel='alpha',
            scenario='upgrade-ab',
            env={
                'KOPS_VERSION_A': "1.24",
                'K8S_VERSION_A': "v1.24.0",
                'KOPS_VERSION_B': "latest",
                'K8S_VERSION_B': "1.25.0",
            }
        ),
        presubmit_test(
            name="pull-kops-e2e-aws-upgrade-k125-kolatest-to-k126-kolatest",
            optional=True,
            distro='u2004',
            networking='cilium',
            k8s_version='stable',
            kops_channel='alpha',
            scenario='upgrade-ab',
            env={
                'KOPS_VERSION_A': "latest",
                'K8S_VERSION_A': "v1.25.0",
                'KOPS_VERSION_B': "latest",
                'K8S_VERSION_B': "v1.26.0",
                'KOPS_SKIP_E2E': '1',
                'KOPS_TEMPLATE': 'tests/e2e/templates/many-addons.yaml.tmpl',
                'KOPS_CONTROL_PLANE_SIZE': '3',
            }
        ),
        presubmit_test(
            name="pull-kops-e2e-aws-upgrade-126-ko126-to-klatest-kolatest-many-addons",
            optional=True,
            distro='u2004',
            networking='cilium',
            k8s_version='stable',
            kops_channel='alpha',
            test_timeout_minutes=120,
            run_if_changed=r'^upup\/(models\/cloudup\/resources\/addons\/|pkg\/fi\/cloudup\/bootstrapchannelbuilder\/)', # pylint: disable=line-too-long
            scenario='upgrade-ab',
            env={
                'KOPS_VERSION_A': "1.26",
                'K8S_VERSION_A': "v1.26.0",
                'KOPS_VERSION_B': "latest",
                'K8S_VERSION_B': "latest",
                'KOPS_SKIP_E2E': '1',
                'KOPS_TEMPLATE': 'tests/e2e/templates/many-addons.yaml.tmpl',
                'KOPS_CONTROL_PLANE_SIZE': '3',
            }
        ),
        presubmit_test(
            name="pull-kops-e2e-aws-upgrade-k123-ko125-to-k124-kolatest-karpenter",
            optional=True,
            distro='u2204arm64',
            networking='cilium',
            k8s_version='stable',
            kops_channel='alpha',
            feature_flags=['Karpenter'],
            test_timeout_minutes=120,
            run_if_changed=r'^upup\/models\/cloudup\/resources\/addons\/karpenter\.sh\/',
            scenario='upgrade-ab',
            extra_flags=[
                "--instance-manager=karpenter",
                "--master-size=c6g.xlarge",
            ],
            env={
                'KOPS_VERSION_A': "1.25",
                'K8S_VERSION_A': "v1.23.0",
                'KOPS_VERSION_B': "latest",
                'K8S_VERSION_B': "v1.24.0",
                'KOPS_SKIP_E2E': '1',
                'KOPS_CONTROL_PLANE_SIZE': '3',
            }
        ),
    ]
    return jobs

########################
# YAML File Generation #
########################
periodics_files = {
    'kops-periodics-conformance.yaml': generate_conformance,
    'kops-periodics-distros.yaml': generate_distros,
    'kops-periodics-grid.yaml': generate_grid,
    'kops-periodics-misc2.yaml': generate_misc,
    'kops-periodics-network-plugins.yaml': generate_network_plugins,
    'kops-periodics-scale.yaml': generate_scale,
    'kops-periodics-upgrades.yaml': generate_upgrades,
    'kops-periodics-versions.yaml': generate_versions,
    'kops-periodics-pipeline.yaml': generate_pipeline,
}

presubmits_files = {
    'kops-presubmits-distros.yaml':generate_presubmits_distros,
    'kops-presubmits-network-plugins.yaml': generate_presubmits_network_plugins,
    'kops-presubmits-e2e.yaml': generate_presubmits_e2e,
    'kops-presubmits-scale.yaml': generate_presubmits_scale,
}

def main():
    for filename, generate_func in periodics_files.items():
        print(f"Generating {filename}")
        output = []
        runs_per_week = 0
        job_count = 0
        for res in generate_func():
            output.append(res[0])
            runs_per_week += res[1]
            job_count += 1
        output.insert(0, "# Test jobs generated by build_jobs.py (do not manually edit)\n")
        output.insert(1, f"# {job_count} jobs, total of {runs_per_week} runs per week\n")
        output.insert(2, "periodics:\n")
        with open(filename, 'w') as fd:
            fd.write(''.join(output))
    for filename, generate_func in presubmits_files.items():
        print(f"Generating {filename}")
        output = []
        job_count = 0
        for res in generate_func():
            output.append(res)
            job_count += 1
        output.insert(0, "# Test jobs generated by build_jobs.py (do not manually edit)\n")
        output.insert(1, f"# {job_count} jobs\n")
        output.insert(2, "presubmits:\n")
        output.insert(3, "  kubernetes/kops:\n")
        with open(filename, 'w') as fd:
            fd.write(''.join(output))

if __name__ == "__main__":
    main()
