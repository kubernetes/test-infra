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
