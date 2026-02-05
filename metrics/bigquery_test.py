#!/usr/bin/env python3

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

"""Tests for bigquery.py"""

import argparse
import json
import os
import re
import shutil
import sys
import tempfile
import unittest

import ruamel.yaml as yaml

import bigquery

class TestBigquery(unittest.TestCase):
    """Tests the bigquery scenario."""

    def test_configs(self):
        """All yaml files in metrics are valid."""
        config_metrics = set()
        for path in bigquery.all_configs(search='**'):
            name = os.path.basename(path)
            if name in ['BUILD', 'BUILD.bazel', 'README.md']:
                continue
            if not path.endswith('.yaml'):
                self.fail('Only .yaml files allowed: %s' % path)

            with open(path) as config_file:
                try:
                    config = yaml.safe_load(config_file)
                except yaml.YAMLError:
                    self.fail(path)
                self.assertIn('metric', config)
                self.assertIn('query', config)
                self.assertIn('jqfilter', config)
                self.assertIn('description', config)
                metric = config['metric'].strip()
                bigquery.validate_metric_name(metric)
                config_metrics.add(metric)
        configs = bigquery.all_configs()
        self.assertTrue(all(p.endswith('.yaml') for p in configs), configs)

        # Check that the '**' search finds the same number of yaml
        # files as the default search.
        self.assertEqual(len(configs), len(config_metrics), "verify the `metric` feild is unique")

        # Check that config files correlate with metrics listed in
        # test-infra/metrics/README.md.
        with open(os.path.join(os.path.dirname(__file__), 'README.md')) as readme_file:
            readme = readme_file.read()

        readme_metrics = set(re.findall(
            r'\(http://storage\.googleapis\.com/k8s-metrics/([\w-]+)-latest\.json\)',
            readme,
        ))
        missing = config_metrics - readme_metrics
        if missing:
            self.fail(
                'test-infra/metrics/README.md is missing entries for metrics: %s.'
                % ', '.join(sorted(missing)),
            )
        extra = readme_metrics - config_metrics
        if extra:
            self.fail(
                'test-infra/metrics/README.md includes metrics that are missing config files: %s.'
                % ', '.join(sorted(extra)),
            )

        # Check that all configs are linked in readme.
        self.assertTrue(all(os.path.basename(p) in readme for p in configs))

    def test_jq(self):
        """do_jq executes a jq filter properly."""
        def check(expr, data, expected):
            """Check a test scenario."""
            with open(self.data_filename, 'w') as data_file:
                data_file.write(data)
            bigquery.do_jq(
                expr,
                self.data_filename,
                self.out_filename,
                jq_bin=ARGS.jq,
            )
            with open(self.out_filename) as out_file:
                actual = out_file.read().replace(' ', '').replace('\n', '')
                self.assertEqual(actual, expected)

        check(
            expr='.',
            data='{ "field": "value" }',
            expected='{"field":"value"}',
        )
        check(
            expr='.field',
            data='{ "field": "value" }',
            expected='"value"',
        )

    def test_validate_metric_name(self):
        """validate_metric_name rejects invalid metric names."""
        bigquery.validate_metric_name('normal')

        def check(test):
            """Check invalid names."""
            self.assertRaises(ValueError, bigquery.validate_metric_name, test)

        check('invalid#metric')
        check('invalid/metric')
        check('in\\valid')
        check('invalid?yes')
        check('*invalid')
        check('[metric]')
        check('metric\n')
        check('met\ric')
        check('metric& invalid')

    def test_collect_valid_jobs(self):
        """collect_valid_jobs extracts job names from Prow config YAML files."""
        # Create a temporary directory with test job configs
        jobs_dir = os.path.join(self.tmpdir, 'jobs')
        os.makedirs(jobs_dir)

        # Create a presubmits config
        presubmits_config = {
            'presubmits': {
                'kubernetes/kubernetes': [
                    {'name': 'pull-kubernetes-unit'},
                    {'name': 'pull-kubernetes-integration'},
                ]
            }
        }
        with open(os.path.join(jobs_dir, 'presubmits.yaml'), 'w') as f:
            yaml.dump(presubmits_config, f)

        # Create a postsubmits config
        postsubmits_config = {
            'postsubmits': {
                'kubernetes/kubernetes': [
                    {'name': 'ci-kubernetes-build'},
                ]
            }
        }
        with open(os.path.join(jobs_dir, 'postsubmits.yaml'), 'w') as f:
            yaml.dump(postsubmits_config, f)

        # Create a periodics config
        periodics_config = {
            'periodics': [
                {'name': 'ci-kubernetes-e2e-gce'},
                {'name': 'ci-kubernetes-verify'},
            ]
        }
        with open(os.path.join(jobs_dir, 'periodics.yaml'), 'w') as f:
            yaml.dump(periodics_config, f)

        valid_jobs = bigquery.collect_valid_jobs(jobs_dir)

        # Check presubmits (both with and without pr: prefix)
        self.assertIn('pull-kubernetes-unit', valid_jobs)
        self.assertIn('pr:pull-kubernetes-unit', valid_jobs)
        self.assertIn('pull-kubernetes-integration', valid_jobs)
        self.assertIn('pr:pull-kubernetes-integration', valid_jobs)

        # Check postsubmits
        self.assertIn('ci-kubernetes-build', valid_jobs)

        # Check periodics
        self.assertIn('ci-kubernetes-e2e-gce', valid_jobs)
        self.assertIn('ci-kubernetes-verify', valid_jobs)

        # Check that non-existent jobs are not included
        self.assertNotIn('non-existent-job', valid_jobs)

    def test_collect_valid_jobs_nonexistent_dir(self):
        """collect_valid_jobs returns empty set for non-existent directory."""
        valid_jobs = bigquery.collect_valid_jobs('/nonexistent/path')
        self.assertEqual(valid_jobs, set())

    def test_collect_valid_jobs_real_config(self):
        """collect_valid_jobs works against real config/jobs directory.

        This integration test ensures the code handles all edge cases present
        in actual job configuration files, including explicit null values,
        empty files, and various YAML structures.
        """
        # Find the real config/jobs directory relative to this test file
        repo_root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
        jobs_dir = os.path.join(repo_root, 'config', 'jobs')

        if not os.path.isdir(jobs_dir):
            self.skipTest('config/jobs directory not found - skipping integration test')

        # This should not raise any exceptions
        valid_jobs = bigquery.collect_valid_jobs(jobs_dir)

        # Sanity check: we should find a reasonable number of jobs
        self.assertGreater(len(valid_jobs), 100,
            'Expected to find more than 100 jobs in config/jobs')

        # Verify some jobs have the pr: prefix (presubmits)
        pr_jobs = [j for j in valid_jobs if j.startswith('pr:')]
        self.assertGreater(len(pr_jobs), 0,
            'Expected to find presubmit jobs with pr: prefix')

    def test_collect_valid_jobs_null_values(self):
        """collect_valid_jobs handles YAML files with explicit null values."""
        # Create a temporary directory with test job configs
        jobs_dir = os.path.join(self.tmpdir, 'jobs')
        os.makedirs(jobs_dir)

        # Create a config with explicit null values (common in real configs)
        # This tests the case where config.get('postsubmits') returns None
        null_config = {
            'presubmits': None,
            'postsubmits': None,
            'periodics': [
                {'name': 'ci-valid-periodic'},
            ]
        }
        with open(os.path.join(jobs_dir, 'null_values.yaml'), 'w') as f:
            yaml.dump(null_config, f)

        # Create a config with only some fields present
        partial_config = {
            'presubmits': {
                'org/repo': [
                    {'name': 'pull-test'},
                ]
            }
            # postsubmits and periodics are missing entirely
        }
        with open(os.path.join(jobs_dir, 'partial.yaml'), 'w') as f:
            yaml.dump(partial_config, f)

        # Create an empty config (yaml.safe_load returns None)
        with open(os.path.join(jobs_dir, 'empty.yaml'), 'w') as f:
            f.write('')

        valid_jobs = bigquery.collect_valid_jobs(jobs_dir)

        # Check jobs from null_values.yaml
        self.assertIn('ci-valid-periodic', valid_jobs)

        # Check jobs from partial.yaml
        self.assertIn('pull-test', valid_jobs)
        self.assertIn('pr:pull-test', valid_jobs)

    def test_filter_json_by_jobs_object_format(self):
        """filter_json_by_jobs filters object-keyed JSON (failures/flakes format)."""
        # Create test JSON with object format
        test_data = {
            'pr:pull-kubernetes-unit': {'failing_days': 5},
            'ci-kubernetes-e2e-gce': {'failing_days': 10},
            'stale-job-removed': {'failing_days': 100},
        }
        json_file = os.path.join(self.tmpdir, 'test.json')
        with open(json_file, 'w') as f:
            json.dump(test_data, f)

        valid_jobs = {'pr:pull-kubernetes-unit', 'ci-kubernetes-e2e-gce'}
        original, remaining = bigquery.filter_json_by_jobs(json_file, valid_jobs)

        self.assertEqual(original, 3)
        self.assertEqual(remaining, 2)

        # Verify the filtered content
        with open(json_file) as f:
            filtered_data = json.load(f)
        self.assertIn('pr:pull-kubernetes-unit', filtered_data)
        self.assertIn('ci-kubernetes-e2e-gce', filtered_data)
        self.assertNotIn('stale-job-removed', filtered_data)

    def test_filter_json_by_jobs_array_format(self):
        """filter_json_by_jobs filters array JSON (job-health format)."""
        # Create test JSON with array format
        test_data = [
            {'job': 'pr:pull-kubernetes-unit', 'runs': 10},
            {'job': 'ci-kubernetes-e2e-gce', 'runs': 20},
            {'job': 'stale-job-removed', 'runs': 5},
        ]
        json_file = os.path.join(self.tmpdir, 'test.json')
        with open(json_file, 'w') as f:
            json.dump(test_data, f)

        valid_jobs = {'pr:pull-kubernetes-unit', 'ci-kubernetes-e2e-gce'}
        original, remaining = bigquery.filter_json_by_jobs(json_file, valid_jobs)

        self.assertEqual(original, 3)
        self.assertEqual(remaining, 2)

        # Verify the filtered content
        with open(json_file) as f:
            filtered_data = json.load(f)
        self.assertEqual(len(filtered_data), 2)
        job_names = [item['job'] for item in filtered_data]
        self.assertIn('pr:pull-kubernetes-unit', job_names)
        self.assertIn('ci-kubernetes-e2e-gce', job_names)
        self.assertNotIn('stale-job-removed', job_names)

    def setUp(self):
        self.assertTrue(ARGS.jq)
        self.tmpdir = tempfile.mkdtemp(prefix='bigquery_test_')
        self.out_filename = os.path.join(self.tmpdir, 'out.json')
        self.data_filename = os.path.join(self.tmpdir, 'data.json')

    def tearDown(self):
        shutil.rmtree(self.tmpdir)

if __name__ == '__main__':
    PARSER = argparse.ArgumentParser()
    PARSER.add_argument('--jq', default='jq', help='path to jq binary')
    ARGS, _ = PARSER.parse_known_args()

    # Remove the --jq flag and its value so that unittest.main can parse the remaining args without
    # complaining about an unknown flag.
    for i in range(len(sys.argv)):
        if sys.argv[i].startswith('--jq'):
            arg = sys.argv.pop(i)
            if '=' not in arg:
                del sys.argv[i]
            break

    unittest.main()
