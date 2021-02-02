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

# Need to figure out why this only fails on travis
# pylint: disable=bad-continuation

"""Dig through jobs/FOO.env, and execute a janitor pass for each of the project"""

import argparse
import json
import os
import re
import subprocess
import sys

try:
    from junit_xml import TestSuite, TestCase
    HAS_JUNIT = True
except ImportError:
    HAS_JUNIT = False

ORIG_CWD = os.getcwd()  # Checkout changes cwd

def test_infra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)


def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)


def parse_project(path):
    """Parse target env file and return GCP project name."""
    with open(path, 'r') as fp:
        env = fp.read()
    match = re.search(r'PROJECT=([^\n"]+)', env)
    if match:
        project = match.group(1)
        return project
    return None


def clean_project(project, hours=24, dryrun=False, ratelimit=None, filt=None):
    """Execute janitor for target GCP project """
    # Multiple jobs can share the same project, woooo
    if project in CHECKED:
        return
    CHECKED.add(project)

    cmd = ['python', test_infra('boskos/cmd/janitor/gcp_janitor.py'), '--project=%s' % project]
    cmd.append('--hour=%d' % hours)
    if dryrun:
        cmd.append('--dryrun')
    if ratelimit:
        cmd.append('--ratelimit=%d' % ratelimit)
    if VERBOSE:
        cmd.append('--verbose')
    if filt:
        cmd.append('--filter=%s' % filt)

    try:
        check(*cmd)
    except subprocess.CalledProcessError:
        FAILED.append(project)


EXEMPT_PROJECTS = [
    'kubernetes-scale', # Let it's up/down job handle the resources
    'k8s-scale-testing', # As it can be running some manual experiments
    'k8s-jkns-e2e-gce-f8n-1-7', # federation projects should use fedtidy to clean up
    'k8s-jkns-e2e-gce-f8n-1-8', # federation projects should use fedtidy to clean up
]

PR_PROJECTS = {
    # k8s-jkns-pr-bldr-e2e-gce-fdrtn
    # k8s-jkns-pr-cnry-e2e-gce-fdrtn
    # cleans up resources older than 3h
    # which is more than enough for presubmit jobs to finish.
    'k8s-jkns-pr-gce': 3,
    'k8s-jkns-pr-gce-bazel': 3,
    'k8s-jkns-pr-gce-etcd3': 3,
    'k8s-jkns-pr-gci-gce': 3,
    'k8s-jkns-pr-gci-gke': 3,
    'k8s-jkns-pr-gci-kubemark': 3,
    'k8s-jkns-pr-gke': 3,
    'k8s-jkns-pr-kubeadm': 3,
    'k8s-jkns-pr-kubemark': 3,
    'k8s-jkns-pr-node-e2e': 3,
    'k8s-jkns-pr-gce-gpus': 3,
}

SCALE_PROJECT = {
    'k8s-presubmit-scale': 3,
}

def check_predefine_jobs(jobs, ratelimit):
    """Handle predefined jobs"""
    for project, expire in jobs.iteritems():
        clean_project(project, hours=expire, ratelimit=ratelimit)

def check_ci_jobs():
    """Handle CI jobs"""
    with open(test_infra('jobs/config.json')) as fp:
        config = json.load(fp)

    match_re = re.compile(r'--gcp-project=(.+)')
    for value in config.values():
        clean_hours = 24
        found = None
        for arg in value.get('args', []):
            # lifetime for soak cluster should be 7 days
            # clean up everything older than 10 days to prevent leak
            if '--soak' in arg:
                clean_hours = 24 * 10
            mat = match_re.match(arg)
            if not mat:
                continue
            project = mat.group(1)
            if any(b in project for b in EXEMPT_PROJECTS):
                print >>sys.stderr, 'Project %r is exempted in ci-janitor' % project
                continue
            if project in PR_PROJECTS or project in SCALE_PROJECT:
                continue # CI janitor skips all PR jobs
            found = project
        if found:
            clean_project(found, clean_hours)


def main(mode, ratelimit, projects, age, artifacts, filt):
    """Run janitor for each project."""
    if mode == 'pr':
        check_predefine_jobs(PR_PROJECTS, ratelimit)
    elif mode == 'scale':
        check_predefine_jobs(SCALE_PROJECT, ratelimit)
    elif mode == 'custom':
        projs = str.split(projects, ',')
        for proj in projs:
            clean_project(proj.strip(), hours=age, ratelimit=ratelimit, filt=filt)
    else:
        check_ci_jobs()

    # Summary
    print 'Janitor checked %d project, %d failed to clean up.' % (len(CHECKED), len(FAILED))
    print HAS_JUNIT
    if artifacts:
        output = os.path.join(artifacts, 'junit_janitor.xml')
        if not HAS_JUNIT:
            print 'Please install junit-xml (https://pypi.org/project/junit-xml/)'
        else:
            print 'Generating junit output:'
            tcs = []
            for project in CHECKED:
                tc = TestCase(project, 'kubernetes_janitor')
                if project in FAILED:
                    # TODO(krzyzacy): pipe down stdout here as well
                    tc.add_failure_info('failed to clean up gcp project')
                tcs.append(tc)

            ts = TestSuite('janitor', tcs)
            with open(output, 'w') as f:
                TestSuite.to_file(f, [ts])
    if FAILED:
        print >>sys.stderr, 'Failed projects: %r' % FAILED
        exit(1)


if __name__ == '__main__':
    # keep some metric
    CHECKED = set()
    FAILED = []
    VERBOSE = False
    PARSER = argparse.ArgumentParser()
    PARSER.add_argument(
        '--mode', default='ci', choices=['ci', 'pr', 'scale', 'custom'],
        help='Which type of projects to clear')
    PARSER.add_argument(
        '--ratelimit', type=int,
        help='Max number of resources to clear in one gcloud delete call '
             '(passed into gcp_janitor.py)')
    PARSER.add_argument(
        '--projects', type=str,
        help='Comma separated list of projects to clean up. Only applicable in custom mode.')
    PARSER.add_argument(
        '--age', type=int,
        help='Expiry age for projects, in hours. Only applicable in custom mode.')
    PARSER.add_argument(
        '--verbose', action='store_true',
        help='If want more detailed logs from the janitor script.')
    PARSER.add_argument(
        '--artifacts',
        help='generate junit style xml to target path',
        default=os.environ.get('ARTIFACTS', None))
    PARSER.add_argument(
        '--filter',
        default=None,
        help='Filter down to these instances(passed into gcp_janitor.py)')
    ARGS = PARSER.parse_args()
    VERBOSE = ARGS.verbose
    main(ARGS.mode, ARGS.ratelimit, ARGS.projects, ARGS.age, ARGS.artifacts, ARGS.filter)
