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

"""Runs bazel build/test for current repo."""

import argparse
import os
import subprocess
import sys

ORIG_CWD = os.getcwd()

def test_infra(*paths):
    """Return path relative to root of test-infra repo."""
    return os.path.join(ORIG_CWD, os.path.dirname(__file__), '..', *paths)

def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)

def check_output(*cmd):
    """Log and run the command, raising on errors, return output"""
    print >>sys.stderr, 'Run:', cmd
    return subprocess.check_output(cmd)


class Bazel(object):
    def __init__(self, batch):
        self.batch = batch

    def check(self, *cmd):
        """wrapper for check('bazel', *cmd) that respects batch"""
        if self.batch:
            check('bazel', '--batch', *cmd)
        else:
            check('bazel', *cmd)

    def check_output(self, *cmd):
        """wrapper for check_output('bazel', *cmd) that respects batch"""
        if self.batch:
            return check_output('bazel', '--batch', *cmd)
        return check_output('bazel', *cmd)

    def query(self, kind, selected_pkgs, changed_pkgs):
        """
        Run a bazel query against target kind, include targets from args.

        Returns a list of kind objects from bazel query.
        """

        # Changes are calculated and no packages found, return empty list.
        if changed_pkgs == []:
            return []

        selection = '//...'
        if selected_pkgs:
            # targets without a '-' operator prefix are implicitly additive
            # when specifying build targets
            selection = selected_pkgs[0]
            for pkg in selected_pkgs[1:]:
                if pkg.startswith('-'):
                    selection += ' '+pkg
                else:
                    selection += ' +'+pkg


        changes = '//...'
        if changed_pkgs:
            changes = 'set(%s)' % ' '.join(changed_pkgs)

        query_pat = 'kind(%s, rdeps(%s, %s)) except attr(\'tags\', \'manual\', //...)'
        return filter(None, self.check_output(
            'query',
            '--keep_going',
            '--noshow_progress',
            query_pat % (kind, selection, changes)
        ).split('\n'))


def upload_string(gcs_path, text):
    """Uploads text to gcs_path"""
    cmd = ['gsutil', '-q', '-h', 'Content-Type:text/plain', 'cp', '-', gcs_path]
    print >>sys.stderr, 'Run:', cmd, 'stdin=%s'%text
    proc = subprocess.Popen(cmd, stdin=subprocess.PIPE)
    proc.communicate(input=text)

def echo_result(res):
    """echo error message bazed on value of res"""
    echo_map = {
        0:'Success',
        1:'Build failed',
        2:'Bad environment or flags',
        3:'Build passed, tests failed or timed out',
        4:'Build passed, no tests found',
        5:'Interrupted'
    }
    print echo_map.get(res, 'Unknown exit code : %s' % res)

def get_version():
    """Return kubernetes version"""
    with open('bazel-genfiles/version') as fp:
        return fp.read().strip()

def get_changed(base, pull):
    """Get affected packages between base sha and pull sha."""
    diff = check_output(
        'git', 'diff', '--name-only',
        '--diff-filter=d', '%s...%s' % (base, pull))
    return check_output(
        'bazel', 'query',
        '--noshow_progress',
        'set(%s)' % diff).split('\n')

def clean_file_in_dir(dirname, filename):
    """Recursively remove all file with filename in dirname."""
    for parent, _, filenames in os.walk(dirname):
        for name in filenames:
            if name == filename:
                os.remove(os.path.join(parent, name))

def main(args):
    """Trigger a bazel build/test run, and upload results."""
    # pylint:disable=too-many-branches, too-many-statements, too-many-locals
    if args.install:
        for install in args.install:
            if not os.path.isfile(install):
                raise ValueError('Invalid install path: %s' % install)
            check('pip', 'install', '-r', install)

    bazel = Bazel(args.batch)

    bazel.check('version')
    res = 0
    try:
        affected = None
        if args.affected:
            base = os.getenv('PULL_BASE_SHA', '')
            pull = os.getenv('PULL_PULL_SHA', 'HEAD')
            if not base:
                raise ValueError('PULL_BASE_SHA must be set!')
            affected = get_changed(base, pull)

        build_pkgs = []
        manual_build_targets = []
        test_pkgs = []
        manual_test_targets = []
        if args.build:
            build_pkgs = args.build.split(' ')
        if args.manual_build:
            manual_build_targets = args.manual_build.split(' ')
        if args.test:
            test_pkgs = args.test.split(' ')
        if args.manual_test:
            manual_test_targets = args.manual_test.split(' ')

        buildables = []
        if build_pkgs or manual_build_targets or affected:
            buildables = bazel.query('.*_binary', build_pkgs, affected) + manual_build_targets

        if buildables:
            bazel.check('build', *buildables)
        else:
            # Call bazel build regardless, to establish bazel symlinks
            bazel.check('build')

        # clean up previous test.xml
        clean_file_in_dir('./bazel-testlogs', 'test.xml')

        if args.release:
            bazel.check('build', *args.release.split(' '))

        if test_pkgs or manual_test_targets or affected:
            tests = bazel.query('test', test_pkgs, affected) + manual_test_targets
            if tests:
                if args.test_args:
                    tests = args.test_args + tests
                bazel.check('test', *tests)
    except subprocess.CalledProcessError as exp:
        res = exp.returncode

    if args.release and res == 0:
        version = get_version()
        if not version:
            print 'Kubernetes version missing; not uploading ci artifacts.'
            res = 1
        else:
            try:
                if args.version_suffix:
                    version += args.version_suffix
                gcs_build = '%s/%s' % (args.gcs, version)
                bazel.check('run', '//:push-build', '--', gcs_build)
                # log push-build location to path child jobs can find
                # (gs://<shared-bucket>/$PULL_REFS/bazel-build-location.txt)
                pull_refs = os.getenv('PULL_REFS', '')
                gcs_shared = os.path.join(args.gcs_shared, pull_refs, 'bazel-build-location.txt')
                if pull_refs:
                    upload_string(gcs_shared, gcs_build)
                if args.publish_version:
                    upload_string(args.publish_version, version)
            except subprocess.CalledProcessError as exp:
                res = exp.returncode

    # Coalesce test results into one file for upload.
    check(test_infra('hack/coalesce.py'))

    echo_result(res)
    if res != 0:
        sys.exit(res)


def create_parser():
    """Create argparser."""
    parser = argparse.ArgumentParser()
    parser.add_argument(
        '--affected', action='store_true',
        help='If build/test affected targets. Filtered by --build and --test flags.')
    parser.add_argument(
        '--build', help='Bazel build target patterns, split by one space')
    parser.add_argument(
        '--manual-build',
        help='Bazel build targets that should always be manually included, split by one space'
    )
    # TODO(krzyzacy): Convert to bazel build rules
    parser.add_argument(
        '--install', action="append", help='Python dependency(s) that need to be installed')
    parser.add_argument(
        '--release', help='Run bazel build, and push release build to --gcs bucket')
    parser.add_argument(
        '--gcs-shared',
        default="gs://kubernetes-jenkins/shared-results/",
        help='If $PULL_REFS is set push build location to this bucket')
    parser.add_argument(
        '--publish-version',
        help='publish GCS file here with the build version, like ci/latest.txt',
    )
    parser.add_argument(
        '--test', help='Bazel test target patterns, split by one space')
    parser.add_argument(
        '--manual-test',
        help='Bazel test targets that should always be manually included, split by one space'
    )
    parser.add_argument(
        '--test-args', action="append", help='Bazel test args')
    parser.add_argument(
        '--gcs',
        default='gs://kubernetes-release-dev/bazel',
        help='GCS path for where to push build')
    parser.add_argument(
        '--version-suffix',
        help='version suffix for build pushing')
    parser.add_argument(
        '--batch', action='store_true', help='run Bazel in batch mode')
    return parser

def parse_args(args=None):
    """Return parsed args."""
    parser = create_parser()
    return parser.parse_args(args)

if __name__ == '__main__':
    main(parse_args())
