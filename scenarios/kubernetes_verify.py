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

# Need to figure out why this only fails on travis
# pylint: disable=bad-continuation

"""Runs verify/test-go checks for kubernetes/kubernetes."""

import argparse
import os
import re
import subprocess
import sys

# This is deprecated from 1.14 onwards.
VERSION_TAG = {
    '1.11': '1.11-v20190318-2ac98e338',
    '1.12': '1.12-v20190318-2ac98e338',
    '1.13': '1.13-v20190817-cc05229',
    '1.14': '1.14-v20190817-cc05229',
    # this is master, feature branches...
    'default': '1.14-v20190817-cc05229',
}


def check_output(*cmd):
    """Log and run the command, return output, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    return subprocess.check_output(cmd)


def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)


def retry(func, times=5):
    """call func until it returns true at most times times"""
    success = False
    for _ in range(0, times):
        success = func()
        if success:
            return success
    return success


def try_call(cmds):
    """returns true if check(cmd) does not throw an exception
    over all cmds where cmds = [[cmd, arg, arg2], [cmd2, arg]]"""
    try:
        for cmd in cmds:
            check(*cmd)
        return True
    # pylint: disable=bare-except
    except:
        return False


def get_git_cache(k8s):
    git = os.path.join(k8s, ".git")
    if not os.path.isfile(git):
        return None
    with open(git) as git_file:
        return git_file.read().replace("gitdir: ", "").rstrip("\n")


def branch_to_tag(branch):
    verify_branch = re.match(r'release-(\d+\.\d+)', branch)
    key = 'default'
    if verify_branch and verify_branch.group(1) in VERSION_TAG:
        key = verify_branch.group(1)
    return VERSION_TAG[key]


def main(branch, script, force, on_prow, exclude_typecheck, exclude_godep, exclude_files_remake):
    """Test branch using script, optionally forcing verify checks."""
    tag = branch_to_tag(branch)

    force = 'y' if force else 'n'
    exclude_typecheck = 'y' if exclude_typecheck else 'n'
    exclude_godep = 'y' if exclude_godep else 'n'
    exclude_files_remake = 'y' if exclude_files_remake else 'n'
    artifacts = '%s/_artifacts' % os.environ['WORKSPACE']
    k8s = os.getcwd()
    if not os.path.basename(k8s) == 'kubernetes':
        raise ValueError(k8s)

    check('rm', '-rf', '.gsutil')
    remote = 'bootstrap-upstream'
    uri = 'https://github.com/kubernetes/kubernetes.git'

    current_remotes = check_output('git', 'remote')
    if re.search('^%s$' % remote, current_remotes, flags=re.MULTILINE):
        check('git', 'remote', 'remove', remote)
    check('git', 'remote', 'add', remote, uri)
    check('git', 'remote', 'set-url', '--push', remote, 'no_push')
    # If .git is cached between runs this data may be stale
    check('git', 'fetch', remote)

    if not os.path.isdir(artifacts):
        os.makedirs(artifacts)

    if on_prow:
        # TODO: on prow REPO_DIR should be /go/src/k8s.io/kubernetes
        # however these paths are brittle enough as is...
        git_cache = get_git_cache(k8s)
        cmd = [
            'docker', 'run', '--rm=true', '--privileged=true',
            '-v', '/var/run/docker.sock:/var/run/docker.sock',
            '-v', '/etc/localtime:/etc/localtime:ro',
            '-v', '%s:/go/src/k8s.io/kubernetes' % k8s,
        ]
        if git_cache is not None:
            cmd.extend(['-v', '%s:%s' % (git_cache, git_cache)])
        cmd.extend([
            '-v', '/workspace/k8s.io/:/workspace/k8s.io/',
            '-v', '%s:/workspace/artifacts' % artifacts,
            '-e', 'KUBE_FORCE_VERIFY_CHECKS=%s' % force,
            '-e', 'KUBE_VERIFY_GIT_BRANCH=%s' % branch,
            '-e', 'EXCLUDE_TYPECHECK=%s' % exclude_typecheck,
            '-e', 'EXCLUDE_FILES_REMAKE=%s' % exclude_files_remake,
            '-e', 'EXCLUDE_GODEP=%s' % exclude_godep,
            '-e', 'REPO_DIR=%s' % k8s,  # hack/lib/swagger.sh depends on this
            '--tmpfs', '/tmp:exec,mode=1777',
            'gcr.io/k8s-testimages/kubekins-test:%s' % tag,
            'bash', '-c', 'cd kubernetes && %s' % script,
        ])
        check(*cmd)
    else:
        check(
            'docker', 'run', '--rm=true', '--privileged=true',
            '-v', '/var/run/docker.sock:/var/run/docker.sock',
            '-v', '/etc/localtime:/etc/localtime:ro',
            '-v', '%s:/go/src/k8s.io/kubernetes' % k8s,
            '-v', '%s:/workspace/artifacts' % artifacts,
            '-e', 'KUBE_FORCE_VERIFY_CHECKS=%s' % force,
            '-e', 'KUBE_VERIFY_GIT_BRANCH=%s' % branch,
            '-e', 'EXCLUDE_TYPECHECK=%s' % exclude_typecheck,
            '-e', 'EXCLUDE_FILES_REMAKE=%s' % exclude_files_remake,
            '-e', 'EXCLUDE_GODEP=%s' % exclude_godep,
            '-e', 'REPO_DIR=%s' % k8s,  # hack/lib/swagger.sh depends on this
            'gcr.io/k8s-testimages/kubekins-test:%s' % tag,
            'bash', '-c', 'cd kubernetes && %s' % script,
        )


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        'Runs verification checks on the kubernetes repo')
    PARSER.add_argument(
        '--branch', default='master', help='Upstream target repo')
    PARSER.add_argument(
        '--force', action='store_true', help='Force all verify checks')
    PARSER.add_argument(
        '--exclude-typecheck', action='store_true', help='Exclude typecheck from verify')
    PARSER.add_argument(
        '--exclude-godep', action='store_true', help='Exclude godep checks from verify')
    PARSER.add_argument(
        '--exclude-files-remake', action='store_true', help='Exclude files remake from verify')
    PARSER.add_argument(
        '--script',
        default='./hack/jenkins/test-dockerized.sh',
        help='Script in kubernetes/kubernetes that runs checks')
    PARSER.add_argument(
        '--prow', action='store_true', help='Force Prow mode'
    )
    ARGS = PARSER.parse_args()
    main(ARGS.branch, ARGS.script, ARGS.force, ARGS.prow,
         ARGS.exclude_typecheck, ARGS.exclude_godep, ARGS.exclude_files_remake)
