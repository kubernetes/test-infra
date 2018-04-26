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

"""Check properties of test projects, optionally fixing them.

Example config:

  {
    "IAM": {
      "roles/editor": [
        "serviceAccount:foo@bar.com",
        "user:me@you.com",
        "group:whatever@googlegroups.com"],
      "roles/viewer": [...],
    },
    "EnableVmZonalDNS': True,
  }
"""

import argparse
import collections
import json
import logging
import os.path
import re
import subprocess
import sys
import threading

from argparse import RawTextHelpFormatter
import yaml

# pylint: disable=invalid-name
_log = logging.getLogger('check_project')

DEFAULT = {
    'IAM': {
        'roles/editor': [
            'serviceAccount:kubekins@kubernetes-jenkins.iam.gserviceaccount.com',
            'serviceAccount:pr-kubekins@kubernetes-jenkins-pull.iam.gserviceaccount.com'],
    },
    'EnableVMZonalDNS': False,
}


class RateLimitedExec(object):
    """Runs subprocess commands with a rate limit."""

    def __init__(self):
        self.semaphore = threading.Semaphore(10)  # 10 concurrent gcloud calls

    def check_output(self, *args, **kwargs):
        """check_output with rate limit"""
        with self.semaphore:
            return subprocess.check_output(*args, **kwargs)

    def call(self, *args, **kwargs):
        """call with rate limit"""
        with self.semaphore:
            return subprocess.call(*args, **kwargs)


class Results(object):
    """Results of the check run"""

    class Info(object):
        """Per-project information"""
        def __init__(self):
            self.updated = False
            # errors is list of strings describing the error context
            self.errors = []
            # People with owners rights to update error projects
            self.helpers = []

    def __init__(self):
        self.lock = threading.Lock()
        self.projects = {}  # project -> Info

    @property
    def errors(self):
        """Returns set of projects that have errors."""
        return set(key for key in self.projects if self.projects[key].errors)

    @property
    def counts(self):
        """Returns count of (total, updated, error'ed) projects."""
        with self.lock:
            total, updated, errored = (0, 0, 0)
            for info in self.projects.values():
                total += 1
                if info.updated:
                    updated += 1
                if info.errors:
                    errored += 1
            return (total, updated, errored)

    def report_project(self, project):
        with self.lock:
            self.ensure_info(project)

    def report_error(self, project, err):
        with self.lock:
            info = self.ensure_info(project)
            info.errors.append(err)

    def report_updated(self, project):
        with self.lock:
            info = self.ensure_info(project)
            info.updated = True

    def add_helper(self, project, helpers):
        with self.lock:
            info = self.ensure_info(project)
            info.helpers = helpers

    def ensure_info(self, project):
        if project not in self.projects:
            info = Results.Info()
            self.projects[project] = info
        return self.projects[project]


def run_threads_to_completion(threads):
    """Runs the given list of threads to completion."""
    for thread in threads:
        thread.start()
    for thread in threads:
        thread.join()


def parse_args():
    """Returns parsed arguments."""
    parser = argparse.ArgumentParser(
        description=__doc__, formatter_class=RawTextHelpFormatter)
    parser.add_argument(
        '--boskos', action='store_true',
        help='If need to check boskos projects')
    parser.add_argument(
        '--filter', default=r'^.+$',
        help='Only look for jobs with the specified names')
    parser.add_argument(
        '--fix', action='store_true', help='Add missing memberships')
    parser.add_argument(
        '--verbose', action='store_true',
        help='Enable verbose output')
    parser.add_argument(
        'config', type=get_config, default='', nargs='?',
        help='Path to json configuration')
    return parser.parse_args()


def get_config(string):
    """Returns configuration for project settings."""
    if not string:
        return DEFAULT
    elif not os.path.isfile(string):
        raise argparse.ArgumentTypeError('not a file: %s' % string)
    with open(string) as fp:
        return json.loads(fp.read())


class Checker(object):
    """Runs the checks against all projects."""

    def __init__(self, config):
        self.config = config
        self.rl_exec = RateLimitedExec()
        self.results = Results()

    def run(self, filt, fix=False, boskos=False):
        """Checks projects for correct settings."""
        def check(project, fix):
            self.results.report_project(project)
            # pylint: disable=no-member
            for prop_class in ProjectProperty.__subclasses__():
                prop = prop_class(self.rl_exec, self.results)
                _log.info('Checking project %s for %s', project, prop.name())
                prop.check_and_maybe_update(self.config, project, fix)

        boskos_path = None
        if boskos:
            boskos_path = '%s/../boskos/resources.yaml' % os.path.dirname(__file__)
        projects = self.load_projects(
            '%s/../jobs/config.json' % os.path.dirname(__file__),
            boskos_path,
            filt)
        _log.info('Checking %d projects', len(projects))

        run_threads_to_completion(
            [threading.Thread(target=check, args=(project, fix))
             for project in sorted(projects)])

        self.log_summary()

    def log_summary(self):
        _log.info('====')
        _log.info(
            'Summary: %d projects, %d have been updated, %d have problems',
            *self.results.counts)
        _log.info('====')

        for key in self.results.errors:
            project = self.results.projects[key]
            _log.info(
                'Project %s needs to fix: %s', key, ','.join(project.errors))

        _log.info('Helpers:')
        fixers = collections.defaultdict(int)
        unk = ['user:unknown']
        for project in self.results.errors:
            helpers = self.results.projects[project].helpers
            if not helpers:
                helpers = unk
            for name in helpers:
                fixers[name] += 1
            _log.info('  %s: %s', project, ','.join(
                self.sane(s) for s in sorted(helpers)))

        for name, count in sorted(fixers.items(), key=lambda i: i[1]):
            _log.info('  %s: %s', count, self.sane(name))

        if self.results.counts[2] != 0:
            sys.exit(1)

    @staticmethod
    def sane(member):
        if ':' not in member:
            raise ValueError(member)
        email = member.split(':')[1]
        return email.split('@')[0]

    @staticmethod
    def load_projects(configs, boskos, filt):
        """Scans the project directories for GCP projects to check."""
        filter_re = re.compile(filt)
        match_re = re.compile(r'--gcp-project=(.+)')
        projects = set()

        with open(configs) as fp:
            config = json.load(fp)

        for job, value in config.iteritems():
            if not filter_re.match(job):
                continue
            for arg in value.get('args', []):
                mat = match_re.match(arg)
                if not mat:
                    continue
                projects.add(mat.group(1))

        if not boskos:
            return projects

        with open(boskos) as fp:
            config = yaml.load(fp.read())
            for rtype in config['resources']:
                if 'project' in rtype['type']:
                    for name in rtype['names']:
                        projects.add(name)

        return projects


class ProjectProperty(object):
    """Base class for properties that are checked for each project.

    Subclasses of this class will be checked against every project.
    """

    def name(self):
        """
        Returns:
            human readable name of the property.
        """
        raise NotImplementedError()

    def check_and_maybe_update(self, config, project, fix):
        """Check and maybe update the project for the required property.

        Args:
            config: project configuration
            project: project to check
            fix: if True, update the project property.
        """
        raise NotImplementedError()


class IAMProperty(ProjectProperty):
    """Project has the correct IAM properties."""

    def __init__(self, rl_exec, results):
        self.rl_exec = rl_exec
        self.results = results

    def name(self):
        return 'IAM'

    def check_and_maybe_update(self, config, project, fix):
        if 'IAM' not in config:
            return

        try:
            out = self.rl_exec.check_output([
                'gcloud',
                'projects',
                'get-iam-policy',
                project,
                '--format=json(bindings)'])
        except subprocess.CalledProcessError:
            _log.info('Cannot access %s', project)
            self.results.report_error(project, 'access')
            return

        needed = config['IAM']
        bindings = json.loads(out)
        fixes = {}
        roles = set()
        for binding in bindings['bindings']:
            role = binding['role']
            roles.add(role)
            members = binding['members']
            if role in needed:
                missing = set(needed[role]) - set(members)
                if missing:
                    fixes[role] = missing
            if role == 'roles/owner':
                self.results.add_helper(project, members)
        missing_roles = set(needed) - roles
        for role in missing_roles:
            fixes[role] = needed[role]

        if not fixes:
            _log.info('Project %s IAM is already configured', project)
            return

        if not fix:
            _log.info('Will not --fix %s, wanted fixed %s', project, fixes)
            self.results.report_error(project, self.name())
            return

        updates = []
        for role, members in sorted(fixes.items()):
            updates.extend(
                threading.Thread(target=self.update, args=(project, role, m))
                for m in members)
        run_threads_to_completion(updates)

    def update(self, project, role, member):
        cmdline = [
            'gcloud', '-q', 'projects', 'add-iam-policy-binding',
            '--role=%s' % role,
            '--member=%s' % member,
            project
        ]
        err = self.rl_exec.call(cmdline, stdout=open('/dev/null', 'w'))
        if not err:
            _log.info('Added %s as %s to %s', member, role, project)
            self.results.report_updated(project)
        else:
            _log.info('Could not update IAM for %s', project)
            self.results.report_error(
                project, 'update %s (role=%s, member=%s)' %
                (self.name(), role, member))


class EnableVmZonalDNS(ProjectProperty):
    """Project has Zonal DNS enabled."""

    def __init__(self, rl_exec, results):
        self.rl_exec = rl_exec
        self.results = results

    def name(self):
        return 'EnableVMZonalDNS'

    def check_and_maybe_update(self, config, project, fix):
        try:
            out = self.rl_exec.check_output([
                'gcloud', 'compute', 'project-info', 'describe',
                '--project=' + project,
                '--format=json(commonInstanceMetadata.items)'])
        except subprocess.CalledProcessError:
            _log.info('Cannot access %s', project)
            return

        enabled = False
        metadata = json.loads(out)
        if (metadata and metadata['commonInstanceMetadata']
                and metadata['commonInstanceMetadata']['items']):
            for item in metadata['commonInstanceMetadata']['items']:
                if item['key'] == 'EnableVmZonalDNS':
                    enabled = item['value'].lower() == 'yes'

        desired = config.get('EnableVMZonalDNS', False)
        if desired == enabled:
            _log.info(
                'Project %s %s is already configured', project, self.name())
            return

        if not fix:
            _log.info(
                'Will not --fix %s, needs to change EnableVMZonalDNS to %s',
                project, desired)
            self.results.report_error(project, self.name())
            return

        if desired != enabled:
            _log.info('Updating project %s EnableVMZonalDNS from %s to %s',
                      project, enabled, desired)
            self.update(project, desired)

    def update(self, project, desired):
        if desired:
            err = self.rl_exec.call(
                ['gcloud', 'compute', 'project-info', 'add-metadata',
                 '--metadata=EnableVmZonalDNS=Yes',
                 '--project=' + project],
                stdout=open('/dev/null', 'w'))
        else:
            err = self.rl_exec.call(
                ['gcloud', 'compute', 'project-info', 'remove-metadata',
                 '--keys=EnableVmZonalDNS',
                 '--project=' + project],
                stdout=open('/dev/null', 'w'))

        if not err:
            _log.info('Updated zonal DNS for %s: %s', project, desired)
            self.results.report_updated(project)
        else:
            _log.info('Could not update zonal DNS for %s', project)
            self.results.report_error(project, 'update ' + self.name())


def main():
    args = parse_args()
    logging.basicConfig(
        format="%(asctime)s %(levelname)s %(name)s] %(message)s",
        level=logging.DEBUG if args.verbose else logging.INFO)
    Checker(args.config).run(args.filter, args.fix, args.boskos)


if __name__ == '__main__':
    main()
