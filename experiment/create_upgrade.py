#!/usr/bin/env python

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

"""Convert Jenkins config into prow config. """

import argparse
import copy
import fileinput
import json
import sys
import yaml

# Always run from test-infra root!
# Sample: ./experiment/create_upgrade.py --upgrade=master --image-old=gci
#         --image-new=container_vm --version-old=1.5 --version-new=master
#         --project=gke-up-g1-5-clat-up-m --testgrid-dashboard=1.5-master-upgrade
#

# pylint: disable=too-many-branches, too-many-statements


TEMPLATE = ('### job-env\n'
            '# Upgrade {DESC}, in {PLATFORM}, from {OLD_IMAGE} {OLD} to {NEW_IMAGE} {NEW}.\n'
            '\n'
            'E2E_OPT=--check_version_skew=false\n'
            'E2E_UPGRADE_TEST=true\n'
            'STORAGE_MEDIA_TYPE=application/vnd.kubernetes.protobuf\n'
            'GINKGO_UPGRADE_TEST_ARGS=--ginkgo.focus=\\[Feature:{FEATURE}}\\] '
            '--upgrade-target={NEW_VERSION} --upgrade-image={NEW_IMAGE}\n'
            'JENKINS_PUBLISHED_SKEW_VERSION={NEW_VERSION}\n'
            'JENKINS_PUBLISHED_VERSION={OLD_VERSION}\n'
            'KUBE_GKE_IMAGE_TYPE={OLD_IMAGE}\n'
            '{SKEW_TEST}\n'
            'PROJECT={PROJECT}\n'
            '\n'
            'KUBEKINS_TIMEOUT={TIMEOUT}\n')

JOB_NAME = ('ci-kubernetes-e2e-{PLATFORM}-{OLD_IMAGE}-{OLD_VERSION}-'
            '{NEW_IMAGE}-{NEW_VERSION}-upgrade-{UPGRADE_TYPE}')

IMAGE_MAP = {
    'container_vm' : 'cvm'
}

VERSION_MAP = {
    'master' : 'ci/latest',
    '1.7' : 'ci/latest-1.7',
    '1.6' : 'ci/latest-1.6',
    '1.5' : 'ci/latest-1.5',
    '1.4' : 'ci/latest-1.4',
}

FEATURE_MAP = {
    'cluster-new' : 'ClusterUpgrade',
    'cluster' : 'ClusterUpgrade',
    'master' : 'MasterUpgrade'
}

DESC_MAP = {
    'cluster-new' : 'master and node',
    'cluster' : 'master and node',
    'master' : 'master only'
}

PROW_CONFIG = {
    'name': '',
    'interval': '6h',
    'spec': {
        'containers': [{
            'image': 'gcr.io/k8s-testimages/kubekins-e2e-prow:v20170418-c08e1094',
            'args': [],
            'volumeMounts': [{
                'readOnly': True,
                'mountPath': '/etc/service-account',
                'name': 'service'
            }, {
                'readOnly': True,
                'mountPath': '/etc/ssh-key-secret',
                'name': 'ssh'
            }, {
                'mountPath': '/root/.cache',
                'name': 'cache-ssd'
            }],
            'env': [{
                'name': 'GOOGLE_APPLICATION_CREDENTIALS',
                'value': '/etc/service-account/service-account.json'
            }, {
                'name': 'USER',
                'value': 'prow'
            }, {
                'name': 'JENKINS_GCE_SSH_PRIVATE_KEY_FILE',
                'value': '/etc/ssh-key-secret/ssh-private'
            }, {
                'name': 'JENKINS_GCE_SSH_PUBLIC_KEY_FILE',
                'value': '/etc/ssh-key-secret/ssh-public'
            }]
        }],
        'volumes': [{
            'secret': {
                'secretName': 'service-account'
            },
            'name': 'service'
        }, {
            'secret': {
                'defaultMode': 256,
                'secretName': 'ssh-key-secret'
            },
            'name': 'ssh'
        }, {
            'hostPath': {
                'path': '/mnt/disks/ssd0'
            },
            'name': 'cache-ssd'
        }]
    }
}

def main(args):
    """goodies."""

    job = JOB_NAME.replace('{PLATFORM}', args.platform)
    job = job.replace('{OLD_IMAGE}', IMAGE_MAP.get(args.image_old, args.image_old))
    job = job.replace('{NEW_IMAGE}', IMAGE_MAP.get(args.image_new, args.image_new))
    job = job.replace('{OLD_VERSION}', args.version_old.replace('.', '-'))
    job = job.replace('{NEW_VERSION}', args.version_new.replace('.', '-'))
    job = job.replace('{UPGRADE_TYPE}', args.upgrade)
    print job

    with open('./jobs/config.json', 'r+') as fp:
        configs = json.loads(fp.read())
        configs[job] = {}
        configs[job]['scenario'] = 'kubernetes_e2e'
        configs[job]['sigOwners'] = ['UNKNOWN']
        configs[job]['args'] = ['--env-file=plarforms/%s.env' % args.platform]
        configs[job]['args'].append('--env-file=jobs/%s.env' % job)
        configs[job]['args'].append('--mode=local')
        fp.seek(0)
        fp.write(json.dumps(configs, sort_keys=True, indent=2))
        fp.write('\n')
        fp.truncate()

    with open('./jobs/%s.env' % job, 'w+') as fp:
        template = TEMPLATE.replace('{PLATFORM}', args.platform)
        template = template.replace('{FEATURE}', FEATURE_MAP.get(args.upgrade))
        template = template.replace('{OLD_IMAGE}', args.image_old)
        template = template.replace('{NEW_IMAGE}', args.image_new)
        template = template.replace('{NEW_VERSION}', VERSION_MAP.get(args.version_new))
        template = template.replace('{OLD_VERSION}', VERSION_MAP.get(args.version_old))
        template = template.replace('{NEW}', args.version_new)
        template = template.replace('{OLD}', args.version_old)
        template = template.replace('{PROJECT}', args.project)
        template = template.replace('{TIMEOUT}', args.timeout)
        template = template.replace('{DESC}', DESC_MAP.get(args.upgrade))
        if args.upgrade == 'cluster-new':
            template = template.replace('{SKEW_TEST}', 'JENKINS_USE_SKEW_TESTS=true')
        else:
            template = template.replace('{SKEW_TEST}', '')
        fp.write(template)

    if args.prow:
        dump = []
        output = copy.deepcopy(PROW_CONFIG)
        output['name'] = job
        prow_args = output['spec']['containers'][0]['args']
        prow_args.append('--timeout=%s' % args.timeout.rstrip('m'))
        prow_args.append('--bare')
        dump.append(output)
        with open(args.prow, 'a') as fp:
            fp.write('\n')
        yaml.safe_dump(dump, file(args.prow, 'a'), default_flow_style=False)

    if args.testgrid:
        testgroup = False
        has_dashboard = False
        tab = False
        next_charm = False
        for line in fileinput.input(args.testgrid, inplace=True):
            if '# Add New Testgroups Here' in line.strip():
                sys.stdout.write('- name: %s\n' % job)
                sys.stdout.write('  gcs_prefix: kubernetes-jenkins/logs/%s\n' % job)
                sys.stdout.write(line)
                testgroup = True
            elif 'name: %s' % args.testgrid_dashboard in line:
                sys.stdout.write(line)
                has_dashboard = True
                next_charm = True
            elif next_charm and line == '\n':
                sys.stdout.write('  - name: %s\n' % job.replace('ci-kubernetes-e2e-', ''))
                sys.stdout.write('    test_group_name: %s\n' % job)
                sys.stdout.write(line)
                next_charm = False
                tab = True
            else:
                sys.stdout.write(line)
        if not testgroup:
            raise ValueError('Fail to append new testgroup', job)
        if not tab:
            if has_dashboard:
                raise ValueError('Fail to append new dashboard tab', job)
            else:
                with open(args.testgrid, 'a') as fp:
                    fp.write('\n')
                    fp.write('- name: %s\n' % args.testgrid_dashboard)
                    fp.write('  dashboard_tab:\n')
                    fp.write('  - name: %s\n' % job.replace('ci-kubernetes-e2e-', ''))
                    fp.write('    test_group_name: %s\n' % job)
                    fp.write('\n')

if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        description='Create an upgrade job')
    PARSER.add_argument(
        '--config',
        help='Path to config.json',
        default='jobs/config.json')
    PARSER.add_argument(
        '--prow',
        help='Path to prow config',
        default='prow/config.yaml')
    PARSER.add_argument(
        '--testgrid',
        help='Path to testgrid config',
        default='testgrid/config/config.yaml')
    PARSER.add_argument(
        '--testgrid-dashboard',
        help='which dashboard to add in testgrid config')
    PARSER.add_argument(
        '--upgrade',
        choices=['master', 'cluster', 'cluster-new'])
    PARSER.add_argument(
        '--platform', help='k8s provider',
        choices=['gce', 'gke'], default='gke')
    PARSER.add_argument(
        '--timeout', help='time limit for the job',
        default='900m')
    PARSER.add_argument(
        '--project', help='designated gcp project for the job',
        default='')
    PARSER.add_argument(
        '--image-old', help='original image source',
        choices=['debian', 'gci', 'container_vm'])
    PARSER.add_argument(
        '--image-new', help='upgraded image source',
        choices=['debian', 'gci', 'container_vm'])
    PARSER.add_argument(
        '--version-old', help='original k8s version',
        choices=['master', '1.7', '1.6', '1.5', '1.4'])
    PARSER.add_argument(
        '--version-new', help='upgraded k8s version',
        choices=['master', '1.7', '1.6', '1.5', '1.4'])

    main(PARSER.parse_args())
