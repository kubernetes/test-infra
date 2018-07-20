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

"""Tests for config.json and Prow configuration."""


import unittest

import collections
import json
import os
import re
import sys

import config_sort
import yaml

# pylint: disable=too-many-public-methods, too-many-branches, too-many-locals, too-many-statements

def get_required_jobs():
    required_jobs = set()
    configs_dir = config_sort.test_infra('mungegithub', 'submit-queue', 'deployment')
    for root, _, files in os.walk(configs_dir):
        for file_name in files:
            if file_name == 'configmap.yaml':
                path = os.path.join(root, file_name)
                with open(path) as fp:
                    conf = yaml.safe_load(fp)
                    for job in conf.get('required-retest-contexts', '').split(','):
                        if job:
                            required_jobs.add(job)
    return required_jobs

class JobTest(unittest.TestCase):

    excludes = [
        'BUILD.bazel',  # For bazel
        'config.json',  # For --json mode
        'validOwners.json', # Contains a list of current sigs; sigs are allowed to own jobs
        'config_sort.py', # Tool script to sort config.json
        'config_test.py', # Script for testing config.json and Prow config.
        'env_gc.py', # Tool script to garbage collect unused .env files.
        'move_extract.py',
    ]
    # also exclude .pyc
    excludes.extend(e + 'c' for e in excludes if e.endswith('.py'))

    prow_config = '../prow/config.yaml'

    realjobs = {}
    prowjobs = []
    presubmits = []

    @property
    def jobs(self):
        """[(job, job_path)] sequence"""
        for path, _, filenames in os.walk(config_sort.test_infra('jobs')):
            print >>sys.stderr, path
            if 'e2e_node' in path:
                # Node e2e image configs, ignore them
                continue
            for job in [f for f in filenames if f not in self.excludes]:
                job_path = os.path.join(path, job)
                yield job, job_path

    def test_config_is_sorted(self):
        """Test jobs/config.json, prow/config.yaml and boskos/resources.yaml are sorted."""
        with open(config_sort.test_infra('jobs/config.json')) as fp:
            original = fp.read()
            expect = config_sort.sorted_job_config().getvalue()
            if original != expect:
                self.fail('jobs/config.json is not sorted, please run '
                          '`bazel run //jobs:config_sort`')
        with open(config_sort.test_infra('prow/config.yaml')) as fp:
            original = fp.read()
            expect = config_sort.sorted_prow_config(
                config_sort.test_infra('prow/config.yaml')).getvalue()
            if original != expect:
                self.fail('prow/config.yaml is not sorted, please run '
                          '`bazel run //jobs:config_sort`')
        with open(config_sort.test_infra('boskos/resources.yaml')) as fp:
            original = fp.read()
            expect = config_sort.sorted_boskos_config().getvalue()
            if original != expect:
                self.fail('boskos/resources.yaml is not sorted, please run '
                          '`bazel run //jobs:config_sort`')

    # TODO(krzyzacy): disabled as we currently have multiple source of truth.
    # We also should migrate shared env files into presets.
    #def test_orphaned_env(self):
    #    orphans = env_gc.find_orphans()
    #    if orphans:
    #        self.fail('the following .env files are not referenced ' +
    #                  'in config.json, please run `bazel run //jobs:env_gc`: ' +
    #                  ' '.join(orphans))

    def check_job_template(self, tmpl):
        builders = tmpl.get('builders')
        if not isinstance(builders, list):
            self.fail(tmpl)
        self.assertEquals(1, len(builders), builders)
        shell = builders[0]
        if not isinstance(shell, dict):
            self.fail(tmpl)
        self.assertEquals(1, len(shell), tmpl)
        if 'raw' in shell:
            self.assertEquals('maintenance-all-{suffix}', tmpl['name'])
            return
        cmd = shell.get('shell')
        if not isinstance(cmd, basestring):
            self.fail(tmpl)
        self.assertIn('--service-account=', cmd)
        self.assertIn('--upload=', cmd)
        if 'kubernetes-security' in cmd:
            self.assertIn('--upload=\'gs://kubernetes-security-jenkins/pr-logs\'', cmd)
        elif '${{PULL_REFS}}' in cmd:
            self.assertIn('--upload=\'gs://kubernetes-jenkins/pr-logs\'', cmd)
        else:
            self.assertIn('--upload=\'gs://kubernetes-jenkins/logs\'', cmd)

    def add_prow_job(self, job):
        name = job.get('name')
        real_job = {}
        real_job['name'] = name
        if 'spec' in job:
            spec = job.get('spec')
            for container in spec.get('containers'):
                if 'args' in container:
                    for arg in container.get('args'):
                        match = re.match(r'[\'\"]?--timeout=(\d+)', arg)
                        if match:
                            real_job['timeout'] = match.group(1)
        if 'pull-' not in name and name in self.realjobs and name not in self.prowjobs:
            self.fail('CI job %s exist in both Jenkins and Prow config!' % name)
        if name not in self.realjobs:
            self.realjobs[name] = real_job
            self.prowjobs.append(name)
        if 'run_after_success' in job:
            for sub in job.get('run_after_success'):
                self.add_prow_job(sub)

    def load_prow_yaml(self, path):
        with open(os.path.join(
            os.path.dirname(__file__), path)) as fp:
            doc = yaml.safe_load(fp)

        if 'periodics' not in doc:
            self.fail('No periodics in prow config!')

        if 'presubmits' not in doc:
            self.fail('No presubmits in prow config!')

        for item in doc.get('periodics'):
            self.add_prow_job(item)

        if 'postsubmits' not in doc:
            self.fail('No postsubmits in prow config!')

        self.presubmits = doc.get('presubmits')
        postsubmits = doc.get('postsubmits')

        for _repo, joblist in self.presubmits.items() + postsubmits.items():
            for job in joblist:
                self.add_prow_job(job)

    def get_real_bootstrap_job(self, job):
        key = os.path.splitext(job.strip())[0]
        if not key in self.realjobs:
            self.load_prow_yaml(self.prow_config)
        self.assertIn(key, sorted(self.realjobs))  # sorted for clearer error message
        return self.realjobs.get(key)

    def test_valid_timeout(self):
        """All e2e jobs has 20min or more container timeout than kubetest timeout."""
        bad_jobs = set()
        with open(config_sort.test_infra('jobs/config.json')) as fp:
            config = json.loads(fp.read())

        for job in config:
            if config.get(job, {}).get('scenario') != 'kubernetes_e2e':
                continue
            realjob = self.get_real_bootstrap_job(job)
            self.assertTrue(realjob)
            self.assertIn('timeout', realjob, job)
            container_timeout = int(realjob['timeout'])

            kubetest_timeout = None
            for arg in config[job]['args']:
                mat = re.match(r'--timeout=(\d+)m', arg)
                if not mat:
                    continue
                kubetest_timeout = int(mat.group(1))
            if kubetest_timeout is None:
                self.fail('Missing timeout: %s' % job)
            if kubetest_timeout > container_timeout:
                bad_jobs.add((job, kubetest_timeout, container_timeout))
            elif kubetest_timeout + 20 > container_timeout:
                bad_jobs.add((
                    'insufficient kubetest leeway',
                    job, kubetest_timeout, container_timeout
                    ))
        if bad_jobs:
            self.fail(
                'jobs: %s, '
                'prow timeout need to be at least 20min longer than timeout in config.json'
                % ('\n'.join(str(s) for s in bad_jobs))
                )

    def test_valid_job_config_json(self):
        """Validate jobs/config.json."""
        # bootstrap integration test scripts
        ignore = [
            'fake-failure',
            'fake-branch',
            'fake-pr',
            'random_job',
        ]

        self.load_prow_yaml(self.prow_config)
        config = config_sort.test_infra('jobs/config.json')
        owners = config_sort.test_infra('jobs/validOwners.json')
        with open(config) as fp, open(owners) as ownfp:
            config = json.loads(fp.read())
            valid_owners = json.loads(ownfp.read())
            for job in config:
                if job not in ignore:
                    self.assertTrue(job in self.prowjobs or job in self.realjobs,
                                    '%s must have a matching jenkins/prow entry' % job)

                # ownership assertions
                self.assertIn('sigOwners', config[job], job)
                self.assertIsInstance(config[job]['sigOwners'], list, job)
                self.assertTrue(config[job]['sigOwners'], job) # non-empty
                owners = config[job]['sigOwners']
                for owner in owners:
                    self.assertIsInstance(owner, basestring, job)
                    self.assertIn(owner, valid_owners, job)

                # env assertions
                self.assertTrue('scenario' in config[job], job)
                scenario = config_sort.test_infra('scenarios/%s.py' % config[job]['scenario'])
                self.assertTrue(os.path.isfile(scenario), job)
                self.assertTrue(os.access(scenario, os.X_OK|os.R_OK), job)
                args = config[job].get('args', [])
                use_shared_build_in_args = False
                extract_in_args = False
                build_in_args = False
                for arg in args:
                    if arg.startswith('--use-shared-build'):
                        use_shared_build_in_args = True
                    elif arg.startswith('--build'):
                        build_in_args = True
                    elif arg.startswith('--extract'):
                        extract_in_args = True
                    match = re.match(r'--env-file=([^\"]+)\.env', arg)
                    if match:
                        env_path = match.group(1)
                        self.assertTrue(env_path.startswith('jobs/'), env_path)
                        path = config_sort.test_infra('%s.env' % env_path)
                        self.assertTrue(
                            os.path.isfile(path),
                            '%s does not exist for %s' % (path, job))
                    elif 'kops' not in job:
                        match = re.match(r'--cluster=([^\"]+)', arg)
                        if match:
                            cluster = match.group(1)
                            self.assertLessEqual(
                                len(cluster), 23,
                                'Job %r, --cluster should be 23 chars or fewer' % job
                                )
                # these args should not be combined:
                # --use-shared-build and (--build or --extract)
                self.assertFalse(use_shared_build_in_args and build_in_args)
                self.assertFalse(use_shared_build_in_args and extract_in_args)
                if config[job]['scenario'] == 'kubernetes_e2e':
                    if job in self.prowjobs:
                        for arg in args:
                            # --mode=local is default now
                            self.assertNotIn('--mode', arg, job)
                    else:
                        self.assertIn('--mode=docker', args, job)
                    for arg in args:
                        if "--env=" in arg:
                            self._check_env(job, arg.split("=", 1)[1])
                    if '--provider=gke' in args:
                        self.assertTrue('--deployment=gke' in args,
                                        '%s must use --deployment=gke' % job)
                        self.assertFalse(any('--gcp-master-image' in a for a in args),
                                         '%s cannot use --gcp-master-image on GKE' % job)
                        self.assertFalse(any('--gcp-nodes' in a for a in args),
                                         '%s cannot use --gcp-nodes on GKE' % job)
                    if '--deployment=gke' in args:
                        self.assertTrue(any('--gcp-node-image' in a for a in args), job)
                    self.assertNotIn('--charts-tests', args)  # Use --charts
                    if any('--check_version_skew' in a for a in args):
                        self.fail('Use --check-version-skew, not --check_version_skew in %s' % job)
                    if '--check-leaked-resources=true' in args:
                        self.fail('Use --check-leaked-resources (no value) in %s' % job)
                    if '--check-leaked-resources==false' in args:
                        self.fail(
                            'Remove --check-leaked-resources=false (default value) from %s' % job)
                    if (
                            '--env-file=jobs/pull-kubernetes-e2e.env' in args
                            and '--check-leaked-resources' in args):
                        self.fail('PR job %s should not check for resource leaks' % job)
                    # Consider deleting any job with --check-leaked-resources=false
                    if (
                            '--provider=gce' not in args
                            and '--provider=gke' not in args
                            and '--check-leaked-resources' in args
                            and 'generated' not in config[job].get('tags', [])):
                        self.fail('Only GCP jobs can --check-leaked-resources, not %s' % job)
                    if '--mode=local' in args:
                        self.fail('--mode=local is default now, drop that for %s' % job)

                    extracts = [a for a in args if '--extract=' in a]
                    shared_builds = [a for a in args if '--use-shared-build' in a]
                    node_e2e = [a for a in args if '--deployment=node' in a]
                    local_e2e = [a for a in args if '--deployment=local' in a]
                    builds = [a for a in args if '--build' in a]
                    if shared_builds and extracts:
                        self.fail(('e2e jobs cannot have --use-shared-build'
                                   ' and --extract: %s %s') % (job, args))
                    elif not extracts and not shared_builds and not node_e2e:
                        # we should at least have --build and --stage
                        if not builds:
                            self.fail(('e2e job needs --extract or'
                                       ' --use-shared-build or'
                                       ' --build: %s %s') % (job, args))

                    if shared_builds or node_e2e:
                        expected = 0
                    elif builds and not extracts:
                        expected = 0
                    elif 'ingress' in job:
                        expected = 1
                    elif any(s in job for s in [
                            'upgrade', 'skew', 'downgrade', 'rollback',
                            'ci-kubernetes-e2e-gce-canary',
                    ]):
                        expected = 2
                    else:
                        expected = 1
                    if len(extracts) != expected:
                        self.fail('Wrong number of --extract args (%d != %d) in %s' % (
                            len(extracts), expected, job))

                    has_image_family = any(
                        [x for x in args if x.startswith('--image-family')])
                    has_image_project = any(
                        [x for x in args if x.startswith('--image-project')])
                    docker_mode = any(
                        [x for x in args if x.startswith('--mode=docker')])
                    if (
                            (has_image_family or has_image_project)
                            and docker_mode):
                        self.fail('--image-family / --image-project is not '
                                  'supported in docker mode: %s' % job)
                    if has_image_family != has_image_project:
                        self.fail('--image-family and --image-project must be'
                                  'both set or unset: %s' % job)

                    if job.startswith('pull-kubernetes-') and not node_e2e and not local_e2e:
                        if 'gke' in job:
                            stage = 'gs://kubernetes-release-dev/ci'
                            suffix = True
                        elif 'kubeadm' in job:
                            # kubeadm-based jobs use out-of-band .deb artifacts,
                            # not the --stage flag.
                            continue
                        else:
                            stage = 'gs://kubernetes-release-pull/ci/%s' % job
                            suffix = False
                        if not shared_builds:
                            self.assertIn('--stage=%s' % stage, args)
                        self.assertEquals(
                            suffix,
                            any('--stage-suffix=' in a for a in args),
                            ('--stage-suffix=', suffix, job, args))


    def test_valid_env(self):
        for job, job_path in self.jobs:
            with open(job_path) as fp:
                data = fp.read()
            if 'kops' in job:  # TODO(fejta): update this one too
                continue
            self.assertNotIn(
                'JENKINS_USE_LOCAL_BINARIES=',
                data,
                'Send --extract=local to config.json, not JENKINS_USE_LOCAL_BINARIES in %s' % job)
            self.assertNotIn(
                'JENKINS_USE_EXISTING_BINARIES=',
                data,
                'Send --extract=local to config.json, not JENKINS_USE_EXISTING_BINARIES in %s' % job)  # pylint: disable=line-too-long

    def test_only_jobs(self):
        """Ensure that everything in jobs/ is a valid job name and script."""
        for job, job_path in self.jobs:
            # Jobs should have simple names: letters, numbers, -, .
            self.assertTrue(re.match(r'[.0-9a-z-_]+.env', job), job)
            # Jobs should point to a real, executable file
            # Note: it is easy to forget to chmod +x
            self.assertTrue(os.path.isfile(job_path), job_path)
            self.assertFalse(os.path.islink(job_path), job_path)
            self.assertTrue(os.access(job_path, os.R_OK), job_path)

    def test_all_project_are_unique(self):
        # pylint: disable=line-too-long
        allowed_list = {
            # The cos image validation jobs intentionally share projects.
            'ci-kubernetes-e2e-gce-cosdev-k8sdev-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sdev-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sdev-slow': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sstable1-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sstable1-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sstable1-slow': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sbeta-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sbeta-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosdev-k8sbeta-slow': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sdev-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sdev-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sdev-slow': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sbeta-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sbeta-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sbeta-slow': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable1-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable1-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable1-slow': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable2-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable2-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable2-slow': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable3-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable3-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosbeta-k8sstable3-slow': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sdev-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sdev-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sdev-slow': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sbeta-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sbeta-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sbeta-slow': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable1-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable1-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable1-slow': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable2-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable2-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable2-slow': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable3-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable3-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2e-gce-cosstable1-k8sstable3-slow': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2enode-cosbeta-k8sdev-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2enode-cosbeta-k8sdev-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2enode-cosbeta-k8sbeta-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2enode-cosbeta-k8sbeta-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2enode-cosbeta-k8sstable1-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2enode-cosbeta-k8sstable1-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2enode-cosbeta-k8sstable2-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2enode-cosbeta-k8sstable2-serial': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2enode-cosbeta-k8sstable3-default': 'ci-kubernetes-e2e-gce-cos*',
            'ci-kubernetes-e2enode-cosbeta-k8sstable3-serial': 'ci-kubernetes-e2e-gce-cos*',

            # The ubuntu image validation jobs intentionally share projects.
            'ci-kubernetes-e2enode-ubuntu1-k8sbeta-gkespec': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu1-k8sbeta-serial': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu1-k8sstable1-gkespec': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu1-k8sstable1-serial': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu1-k8sstable2-gkespec': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu1-k8sstable2-serial': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu1-k8sstable3-gkespec': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu1-k8sstable3-serial': 'ci-kubernetes-e2e-ubuntu-node*',

            'ci-kubernetes-e2e-gce-ubuntu1-k8sbeta-default': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu1-k8sbeta-serial': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu1-k8sbeta-slow': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu1-k8sstable1-default': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu1-k8sstable1-serial': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu1-k8sstable1-slow': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu1-k8sstable2-default': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu1-k8sstable2-serial': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu1-k8sstable2-slow': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu1-k8sstable3-default': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu1-k8sstable3-serial': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu1-k8sstable3-slow': 'ci-kubernetes-e2e-gce-ubuntu*',

            'ci-kubernetes-e2enode-ubuntu2-k8sbeta-gkespec': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu2-k8sbeta-serial': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu2-k8sstable1-gkespec': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu2-k8sstable1-serial': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu2-k8sstable2-gkespec': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu2-k8sstable2-serial': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu2-k8sstable3-gkespec': 'ci-kubernetes-e2e-ubuntu-node*',
            'ci-kubernetes-e2enode-ubuntu2-k8sstable3-serial': 'ci-kubernetes-e2e-ubuntu-node*',

            'ci-kubernetes-e2e-gce-ubuntu2-k8sbeta-default': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu2-k8sbeta-serial': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu2-k8sbeta-slow': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu2-k8sstable1-default': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu2-k8sstable1-serial': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu2-k8sstable1-slow': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu2-k8sstable2-default': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu2-k8sstable2-serial': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu2-k8sstable2-slow': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu2-k8sstable3-default': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu2-k8sstable3-serial': 'ci-kubernetes-e2e-gce-ubuntu*',
            'ci-kubernetes-e2e-gce-ubuntu2-k8sstable3-slow': 'ci-kubernetes-e2e-gce-ubuntu*',

            # The release branch scalability jobs intentionally share projects.
            'ci-kubernetes-e2e-gci-gce-scalability-stable2': 'ci-kubernetes-e2e-gci-gce-scalability-release-*',
            'ci-kubernetes-e2e-gci-gce-scalability-stable1': 'ci-kubernetes-e2e-gci-gce-scalability-release-*',
            'ci-kubernetes-e2e-gce-scalability': 'ci-kubernetes-e2e-gce-scalability-*',
            'ci-kubernetes-e2e-gce-scalability-canary': 'ci-kubernetes-e2e-gce-scalability-*',
            # TODO(fejta): remove these (found while migrating jobs)
            'ci-kubernetes-kubemark-100-gce': 'ci-kubernetes-kubemark-*',
            'ci-kubernetes-kubemark-100-canary': 'ci-kubernetes-kubemark-*',
            'ci-kubernetes-kubemark-5-gce-last-release': 'ci-kubernetes-kubemark-*',
            'ci-kubernetes-kubemark-high-density-100-gce': 'ci-kubernetes-kubemark-*',
            'ci-kubernetes-kubemark-gce-scale': 'ci-kubernetes-scale-*',
            'pull-kubernetes-kubemark-e2e-gce-big': 'ci-kubernetes-scale-*',
            'pull-kubernetes-kubemark-e2e-gce-scale': 'ci-kubernetes-scale-*',
            'pull-kubernetes-e2e-gce-100-performance': 'ci-kubernetes-scale-*',
            'pull-kubernetes-e2e-gce-big-performance': 'ci-kubernetes-scale-*',
            'pull-kubernetes-e2e-gce-large-performance': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gce-large-manual-up': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gce-large-manual-down': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gce-large-correctness': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gce-large-performance': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gce-scale-correctness': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gce-scale-performance': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gke-large-correctness': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gke-large-performance': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gke-large-performance-regional': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gke-large-deploy': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gke-large-teardown': 'ci-kubernetes-scale-*',
            'ci-kubernetes-e2e-gke-scale-correctness': 'ci-kubernetes-scale-*',
            'pull-kubernetes-e2e-gce': 'pull-kubernetes-e2e-gce-*',
            'pull-kubernetes-e2e-gce-canary': 'pull-kubernetes-e2e-gce-*',
            'ci-kubernetes-e2e-gce': 'ci-kubernetes-e2e-gce-*',
            'ci-kubernetes-e2e-gce-canary': 'ci-kubernetes-e2e-gce-*',
            'ci-kubernetes-node-kubelet-serial': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet-orphans': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet-serial-cpu-manager': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet-features': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet-flaky': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet-conformance': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet-benchmark': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet-stable1': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet-stable2': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet-stable3': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet-alpha': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet-beta': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet-beta-features': 'ci-kubernetes-node-kubelet-*',
            'ci-kubernetes-node-kubelet-non-cri-1-6': 'ci-kubernetes-node-kubelet-*',
            # The cri-containerd validation node e2e jobs intentionally share projects.
            'ci-cri-containerd-node-e2e': 'cri-containerd-node-e2e-*',
            'ci-cri-containerd-node-e2e-serial': 'cri-containerd-node-e2e-*',
            'ci-cri-containerd-node-e2e-features': 'cri-containerd-node-e2e-*',
            'ci-cri-containerd-node-e2e-flaky': 'cri-containerd-node-e2e-*',
            'ci-cri-containerd-node-e2e-benchmark': 'cri-containerd-node-e2e-*',
            'ci-containerd-node-e2e': 'cri-containerd-node-e2e-*',
            'ci-containerd-node-e2e-1-1': 'cri-containerd-node-e2e-*',
            'ci-containerd-node-e2e-features': 'cri-containerd-node-e2e-*',
            # ci-cri-containerd-e2e-gce-stackdriver intentionally share projects with
            # ci-kubernetes-e2e-gce-stackdriver.
            'ci-kubernetes-e2e-gce-stackdriver': 'k8s-jkns-e2e-gce-stackdriver',
            'ci-cri-containerd-e2e-gce-stackdriver': 'k8s-jkns-e2e-gce-stackdriver',
            # ingress-GCE e2e jobs
            'pull-ingress-gce-e2e': 'e2e-ingress-gce',
            'ci-ingress-gce-e2e': 'e2e-ingress-gce',
            # sig-autoscaling jobs intentionally share projetcs
            'ci-kubernetes-e2e-gci-gce-autoscaling-hpa':'ci-kubernetes-e2e-gci-gce-autoscaling',
            'ci-kubernetes-e2e-gci-gce-autoscaling-migs-hpa':'ci-kubernetes-e2e-gci-gce-autoscaling-migs',
            'ci-kubernetes-e2e-gci-gke-autoscaling-hpa':'ci-kubernetes-e2e-gci-gke-autoscaling',
            # gpu+autoscaling jobs intentionally share projects with gpu tests
            'ci-kubernetes-e2e-gci-gke-autoscaling-gpu-v100': 'ci-kubernetes-e2e-gke-staging-latest-device-plugin-gpu-v100',
        }
        # pylint: enable=line-too-long
        projects = collections.defaultdict(set)
        boskos = []
        with open(config_sort.test_infra('boskos/resources.yaml')) as fp:
            boskos_config = yaml.safe_load(fp)
            for rtype in boskos_config['resources']:
                if 'project' in rtype['type']:
                    for name in rtype['names']:
                        boskos.append(name)

        with open(config_sort.test_infra('jobs/config.json')) as fp:
            job_config = json.load(fp)
            for job in job_config:
                project = ''
                cfg = job_config.get(job.rsplit('.', 1)[0], {})
                if cfg.get('scenario') == 'kubernetes_e2e':
                    for arg in cfg.get('args', []):
                        if not arg.startswith('--gcp-project='):
                            continue
                        project = arg.split('=', 1)[1]
                if project:
                    if project in boskos:
                        self.fail('Project %s cannot be in boskos/resources.yaml!' % project)
                    projects[project].add(allowed_list.get(job, job))

        duplicates = [(p, j) for p, j in projects.items() if len(j) > 1]
        if duplicates:
            self.fail('Jobs duplicate projects:\n  %s' % (
                '\n  '.join('%s: %s' % t for t in duplicates)))

    def test_jobs_do_not_source_shell(self):
        for job, job_path in self.jobs:
            with open(job_path) as fp:
                script = fp.read()
            self.assertFalse(re.search(r'\Wsource ', script), job)
            self.assertNotIn('\n. ', script, job)

    def _check_env(self, job, setting):
        if not re.match(r'[0-9A-Z_]+=[^\n]*', setting):
            self.fail('[%r]: Env %r: need to follow FOO=BAR pattern' % (job, setting))
        if '#' in setting:
            self.fail('[%r]: Env %r: No inline comments' % (job, setting))
        if '"' in setting or '\'' in setting:
            self.fail('[%r]: Env %r: No quote in env' % (job, setting))
        if '$' in setting:
            self.fail('[%r]: Env %r: Please resolve variables in env' % (job, setting))
        if '{' in setting or '}' in setting:
            self.fail('[%r]: Env %r: { and } are not allowed in env' % (job, setting))
        # also test for https://github.com/kubernetes/test-infra/issues/2829
        # TODO(fejta): sort this list
        black = [
            ('CHARTS_TEST=', '--charts-tests'),
            ('CLUSTER_IP_RANGE=', '--test_args=--cluster-ip-range=FOO'),
            ('CLOUDSDK_BUCKET=', '--gcp-cloud-sdk=gs://foo'),
            ('CLUSTER_NAME=', '--cluster=FOO'),
            ('E2E_CLEAN_START=', '--test_args=--clean-start=true'),
            ('E2E_DOWN=', '--down=true|false'),
            ('E2E_MIN_STARTUP_PODS=', '--test_args=--minStartupPods=FOO'),
            ('E2E_NAME=', '--cluster=whatever'),
            ('E2E_PUBLISH_PATH=', '--publish=gs://FOO'),
            ('E2E_REPORT_DIR=', '--test_args=--report-dir=FOO'),
            ('E2E_REPORT_PREFIX=', '--test_args=--report-prefix=FOO'),
            ('E2E_TEST=', '--test=true|false'),
            ('E2E_UPGRADE_TEST=', '--upgrade_args=FOO'),
            ('E2E_UP=', '--up=true|false'),
            ('E2E_OPT=', 'Send kubetest the flags directly'),
            ('FAIL_ON_GCP_RESOURCE_LEAK=', '--check-leaked-resources=true|false'),
            ('FEDERATION_DOWN=', '--down=true|false'),
            ('FEDERATION_UP=', '--up=true|false'),
            ('GINKGO_PARALLEL=', '--ginkgo-parallel=# (1 for serial)'),
            ('GINKGO_PARALLEL_NODES=', '--ginkgo-parallel=# (1 for serial)'),
            ('GINKGO_TEST_ARGS=', '--test_args=FOO'),
            ('GINKGO_UPGRADE_TEST_ARGS=', '--upgrade_args=FOO'),
            ('JENKINS_FEDERATION_PREFIX=', '--stage=gs://FOO'),
            ('JENKINS_GCI_PATCH_K8S=', 'Unused, see --extract docs'),
            ('JENKINS_PUBLISHED_VERSION=', '--extract=V'),
            ('JENKINS_PUBLISHED_SKEW_VERSION=', '--extract=V'),
            ('JENKINS_USE_SKEW_KUBECTL=', 'SKEW_KUBECTL=y'),
            ('JENKINS_USE_SKEW_TESTS=', '--skew'),
            ('JENKINS_SOAK_MODE', '--soak'),
            ('JENKINS_SOAK_PREFIX', '--stage=gs://FOO'),
            ('JENKINS_USE_EXISTING_BINARIES=', '--extract=local'),
            ('JENKINS_USE_LOCAL_BINARIES=', '--extract=none'),
            ('JENKINS_USE_SERVER_VERSION=', '--extract=gke'),
            ('JENKINS_USE_GCI_VERSION=', '--extract=gci/FAMILY'),
            ('JENKINS_USE_GCI_HEAD_IMAGE_FAMILY=', '--extract=gci/FAMILY'),
            ('KUBE_GKE_NETWORK=', '--gcp-network=FOO'),
            ('KUBE_GCE_NETWORK=', '--gcp-network=FOO'),
            ('KUBE_GCE_ZONE=', '--gcp-zone=FOO'),
            ('KUBEKINS_TIMEOUT=', '--timeout=XXm'),
            ('KUBEMARK_TEST_ARGS=', '--test_args=FOO'),
            ('KUBEMARK_TESTS=', '--test_args=--ginkgo.focus=FOO'),
            ('KUBEMARK_MASTER_SIZE=', '--kubemark-master-size=FOO'),
            ('KUBEMARK_NUM_NODES=', '--kubemark-nodes=FOO'),
            ('KUBE_OS_DISTRIBUTION=', '--gcp-node-image=FOO and --gcp-master-image=FOO'),
            ('KUBE_NODE_OS_DISTRIBUTION=', '--gcp-node-image=FOO'),
            ('KUBE_MASTER_OS_DISTRIBUTION=', '--gcp-master-image=FOO'),
            ('KUBERNETES_PROVIDER=', '--provider=FOO'),
            ('PERF_TESTS=', '--perf'),
            ('PROJECT=', '--gcp-project=FOO'),
            ('SKEW_KUBECTL=', '--test_args=--kubectl-path=FOO'),
            ('USE_KUBEMARK=', '--kubemark'),
            ('ZONE=', '--gcp-zone=FOO'),
        ]
        for env, fix in black:
            if 'kops' in job and env in [
                    'JENKINS_PUBLISHED_VERSION=',
                    'JENKINS_USE_LOCAL_BINARIES=',
                    'GINKGO_TEST_ARGS=',
                    'KUBERNETES_PROVIDER=',
            ]:
                continue  # TODO(fejta): migrate kops jobs
            if setting.startswith(env):
                self.fail('[%s]: Env %s: Convert %s to use %s in jobs/config.json' % (
                    job, setting, env, fix))

    def test_envs_no_export(self):
        for job, job_path in self.jobs:
            if not job.endswith('.env'):
                continue
            with open(job_path) as fp:
                lines = list(fp)
            for line in lines:
                line = line.strip()
                self.assertFalse(line.endswith('\\'))
                if not line:
                    continue
                if line.startswith('#'):
                    continue
                self._check_env(job, line)

    def test_envs_non_empty(self):
        bad = []
        for job, job_path in self.jobs:
            if not job.endswith('.env'):
                continue
            with open(job_path) as fp:
                lines = list(fp)
            for line in lines:
                line = line.strip()
                if line and not line.startswith('#'):
                    break
            else:
                bad.append(job)
        if bad:
            self.fail('%s is empty, please remove the file(s)' % bad)

    def test_no_bad_vars_in_jobs(self):
        """Searches for jobs that contain ${{VAR}}"""
        for job, job_path in self.jobs:
            with open(job_path) as fp:
                script = fp.read()
            bad_vars = re.findall(r'(\${{.+}})', script)
            if bad_vars:
                self.fail('Job %s contains bad bash variables: %s' % (job, ' '.join(bad_vars)))

if __name__ == '__main__':
    unittest.main()
