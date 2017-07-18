#!/usr/bin/env python

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

"""Migrate --extract flag from an JENKINS_FOO env to a scenario flag."""

import json
import os
import re
import sys

ORIG_CWD = os.getcwd()  # Checkout changes cwd

def test_infra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)

def sort():
    """Sort config.json alphabetically."""
    # pylint: disable=too-many-branches,too-many-statements,too-many-locals
    with open(test_infra('jobs/config.json'), 'r+') as fp:
        configs = json.loads(fp.read())
    regexp = re.compile('|'.join([
        r'^KUBERNETES_PROVIDER=(.*)$',
        r'^(?:KUBE_GCE_)?ZONE=(.*)$',
        r'^CLOUDSDK_BUCKET=(.*)$',
        r'^PROJECT=(.*)$',
        r'^KUBEMARK_TESTS=(.*)$',
    ]))
    problems = []
    for job, values in configs.items():
        if values.get('scenario') != 'kubernetes_e2e':
            continue
        if 'args' not in values:
            continue
        args = values['args']
        if any('None' in a for a in args):
            problems.append('Bad flag with None: %s' % job)
            continue
        with open(test_infra('jobs/%s.env' % job)) as fp:
            env = fp.read()
        lines = []
        new_args = {}
        processed = []
        for arg in args:
            if re.search(r'^(--provider|--gcp-zone|--gcp-cloud-sdk|--gcp-project)=', arg):
                key, val = arg.split('=', 1)
                new_args[key] = val
            else:
                processed.append(arg)
            if arg == '--env-file=jobs/platform/gke.env':
                new_args.update({
                    '--provider': 'gke',
                    '--gcp-cloud-sdk': 'gs://cloud-sdk-testing/ci/staging',
                    '--gcp-zone': 'us-central1-f',
                })
            if arg == '--env-file=jobs/platform/gce.env':
                new_args.update({
                    '--provider': 'gce',
                    '--gcp-zone': 'us-central1-f',
                })
            if arg == '--env-file=jobs/ci-kubernetes-e2e-gce-gpu.env':
                new_args.update({
                    '--gcp-zone': 'us-west1-b',
                })
            if arg == '--env-file=jobs/ci-kubernetes-e2e-gke-gpu.env':
                new_args.update({
                    '--gcp-zone': 'us-west1-b',
                })
            if arg == '--env-file=jobs/ci-kubernetes-e2e-gce-canary.env':
                new_args['--gcp-project'] = 'k8s-jkns-e2e-gce'
        args = processed
        okay = False
        for line in env.split('\n'):
            mat = regexp.search(line)
            if not mat:
                lines.append(line)
                continue
            prov, zone, sdk, proj, kubemark = mat.groups()
            stop = False
            for key, val in {
                    '--provider': prov,
                    '--gcp-zone': zone,
                    '--gcp-cloud-sdk': sdk,
                    '--gcp-project': proj,
                    '--test_args': kubemark and '--ginkgo.focus=%s' % kubemark,
            }.items():
                if not val:
                    continue
                new_args[key] = val
            if stop:
                break
        else:
            okay = True
        if not okay:
            continue
        args = list(args)
        for key, val in new_args.items():
            args.append('%s=%s' % (key, val))
        if not any(a.startswith('--provider=') for a in args):
            problems.append('Missing --provider: %s' % job)
            continue
        if '-gce' in job or '-gke' in job:
            if not any(a.startswith('--gcp-zone=') for a in args):
                problems.append('Missing --gcp-zone: %s' % job)
                continue
            if not any(a.startswith('--gcp-project=') for a in args):
                problems.append('Missing --gcp-project: %s' % job)
        flags = set()
        okay = False
        for arg in args:
            try:
                flag, _ = arg.split('=', 1)
            except ValueError:
                flag = ''
            if flag and flag not in ['--env-file', '--extract']:
                if flag in flags:
                    problems.append('Multiple %s in %s' % (flag, job))
                    break
                flags.add(flag)
        else:
            okay = True
        if not okay:
            continue
        values['args'] = args
        with open(test_infra('jobs/%s.env' % job), 'w') as fp:
            fp.write('\n'.join(lines))
    with open(test_infra('jobs/config.json'), 'w') as fp:
        fp.write(json.dumps(configs, sort_keys=True, indent=2, separators=(',', ': ')))
        fp.write('\n')
    if not problems:
        sys.exit(0)
    print >>sys.stderr, '%d problems' % len(problems)
    print '\n'.join(problems)

if __name__ == '__main__':
    sort()
