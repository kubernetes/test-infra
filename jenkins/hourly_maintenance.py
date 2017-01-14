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


import argparse
import collections
import datetime
import glob
import os
import re
import subprocess
import sys
import traceback

import requests

def ContainerImages():
    for line in subprocess.check_output([
        'docker',
        'ps', '-a',
        '--format={{.Image}}']).split('\n'):
      if not line:
          continue
      yield line


def KillContainers():
    """Kill containers that have been running for a long time."""
    now = datetime.datetime.now()
    old = []
    for line in subprocess.check_output([
        'docker', 'ps', '-a',
        '-f', 'status=running',
        '--format={{.CreatedAt}}\t{{.ID}}',
    ]).split('\n'):
        if not line:
            continue
        created, name = line.split('\t')
        fmt = 'YYYY-MM-dd HH:MM'
        dt = datetime.datetime.strptime(created[:len(fmt)], '%Y-%m-%d %H:%M')
        if now - dt > datetime.timedelta(days=1):
            old.append(name)

    if not old:
        return 0

    print 'Old running containers to kill:', old
    err = subprocess.call(['docker', 'kill'] + old)
    if err:
        print >>sys.stderr, 'KillContainers failed'
    return err


def RemoveContainers():
    """Remove non-running containers that we created a long time ago."""
    now = datetime.datetime.now()
    old = []
    for line in subprocess.check_output([
        'docker',
        'ps', '-a',
        '-f', 'status=created',  # Never started due to timeout
        '-f', 'status=exited',  # Container exited
        '-f', 'status=dead',  # Zombie container
        '--format={{.CreatedAt}}\t{{.ID}}\t{{.Image}}',
    ]).split('\n'):
        if not line:
            continue
        created, name, image = line.split('\t')
        fmt = 'YYYY-mm-dd HH:MM'
        dt = datetime.datetime.strptime(created[:len(fmt)], '%Y-%m-%d %H:%M')
        if now - dt > datetime.timedelta(hours=2):
            old.append(name)

    if not old:
        return 0

    print 'Old non-running containers to remove:', old
    err = subprocess.call(['docker', 'rm', '-v'] + old)
    if err:
        print >>sys.stderr, 'RemoveContainers failed'
    return err


def RemoveImages(skip, ancient):
    """Remove all tagged images except the most recently downloaded one."""
    tags = collections.defaultdict(list)
    images = subprocess.check_output(['docker', 'images'])

    for line in images.split('\n'):
        if not line:
            continue
        name, tag, _, age = re.split(r'\s+', line.strip())[:4]
        if 'minutes' in age:
            continue
        if 'hour' in age and 'hours' not in age:
            continue
        if '%s:%s' % (name, tag) in skip:
            continue
        tags[name].append(tag)
        if ancient and ('weeks' in age or 'months' in age):
            tags[name].append(tag)  # Always delete ancient images

    err = 0
    for name, versions in tags.items():
        if name == '<none>':
            continue
        if len(versions) < 2:
            continue
        untag = ['%s:%s' % (name, v) for v in set(versions[1:])]
        print 'Remove %d %s images: %s' % (len(untag), name, untag)
        err |= subprocess.call(['docker', 'rmi'] + untag)

    dangling = subprocess.check_output([
        'docker', 'images', '-q', '-f', 'dangling=true'])
    if dangling:
        err |= subprocess.call(['docker', 'rmi', '-f'] + dangling.split())

    if err:
        print >>sys.stderr, 'RemoveImages failed'
    return err


def RemoveVolumes():
    """Run docker cleanup volumes."""
    err = subprocess.call([
        'docker', 'run',
        '-v', '/var/run/docker.sock:/var/run/docker.sock',
        '-v', '/var/lib/docker:/var/lib/docker',
        '--rm', 'martin/docker-cleanup-volumes'])
    if err:
        print >>sys.stderr, 'RemoveVolumes failed'
    return err


def KillLoopingBash():
    err = 0
    bash_procs = subprocess.check_output(['pgrep', '-f', '^(/bin/)?bash']).split()

    clock_hz = os.sysconf(os.sysconf_names['SC_CLK_TCK'])
    for pid in bash_procs:
        # man 5 proc
        with open('/proc/%s/stat' % pid) as f:
            stat = f.read().split()
        utime = int(stat[13]) / clock_hz
        utime_minutes = utime / 60
        if utime_minutes > 30:
            with open('/proc/%s/cmdline' % pid) as f:
                cmdline = f.read().replace('\x00', ' ').strip()
            print "killing bash pid %s (%r) with %d minutes of CPU time" % (
                pid, cmdline, utime_minutes)
            print 'Environment variables:'
            environ = subprocess.check_output(['sudo', 'cat', '/proc/%s/environ' % pid])
            print '\n'.join(sorted(environ.split('\x00')))
            err |= subprocess.call(['sudo', 'kill', '-9', pid])
    return err


def DeleteCorruptGitRepos():
    """
    Find and delete corrupt .git directories. This can occur when the agent
    reboots in the middle of a git operation. This is *still* less flaky than doing
    full clones every time and occasionally timing out because GitHub is throttling us :(

    Git complains with things like this:

    error: object file ws/.git/objects/01/e6eeca... is empty
    fatal: loose object 01e6eeca211171e9ae5117bbeed738218d2cdb09
        (stored in ws/.git/objects/01/e6eeca..) is corrupt
    """
    # TODO(rmmh): find a way to run this on boot for each jenkins agent, to
    # clean up corrupted git directories before a job can trip over them.
    err = 0
    for git_dir in glob.glob('/var/lib/jenkins/workspace/*/.git'):
        if not subprocess.check_output(['find', git_dir, '-size', '0']):
            # git fsck is kind of slow (~30s each), this fast heuristic speeds things up.
            continue
        print 'validating git dir:', git_dir
        corrupt = subprocess.call(['git', '--git-dir', git_dir, 'fsck'])
        err |= corrupt  # flag
        if err:
            print 'deleting corrupt git dir'
            err |= subprocess.call(['rm', '-rf', git_dir])
    return err


def DeleteOldWorkspaces():
    err = 0
    live_jobs = None
    for host in ('jenkins-master', 'pull-jenkins-master'):
        try:
            resp = requests.get("http://%s:8080/api/json?pretty=true&tree=jobs[name]" % host)
            resp.raise_for_status()
            live_jobs = {job['name'] for job in resp.json()['jobs']}
        except requests.exceptions.ConnectionError:
            continue
        except requests.exceptions.RequestException:
            traceback.print_exc()
    if live_jobs is None:
        print 'unable to determine live jenkins jobs, not deleting any workspaces'
        return 1
    for dirname in sorted(glob.glob('/var/lib/jenkins/workspace/*')):
        key = os.path.basename(dirname).replace('@tmp', '')
        if key not in live_jobs:
            print 'deleting old job workspace', dirname
            err |= subprocess.call(['rm', '-rf', dirname])
    return err


def main(ancient):
    # Copied from http://blog.yohanliyanage.com/2015/05/docker-clean-up-after-yourself/
    err = 0
    err |= KillContainers()
    err |= RemoveContainers()
    err |= RemoveImages(set(ContainerImages()), ancient)
    err |= RemoveVolumes()
    err |= KillLoopingBash()
    err |= DeleteCorruptGitRepos()
    err |= DeleteOldWorkspaces()
    sys.exit(err)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(
        description='Run hourly maintenance on jenkins agents')
    parser.add_argument('--ancient', action='store_true', help='Delete all old images')
    args = parser.parse_args()
    main(args.ancient)
