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

"""Tests for bigquery.py"""

import argparse
import os
import shutil
import sys
import tempfile
import unittest

import yaml

import bigquery

class TestBigquery(unittest.TestCase):
    """Tests the bigquery scenario."""

    def test_configs(self):
        """All yaml files in metrics are valid."""
        for path in bigquery.all_configs(search='**'):
            name = os.path.basename(path)
            if name in ['BUILD', 'README.md']:
                continue
            if not path.endswith('.yaml'):
                self.fail('Only .yaml files allowed: %s' % path)

            with open(path) as config_file:
                config = yaml.safe_load(config_file)
                if not config:
                    self.fail(path)
                self.assertIn('metric', config)
                self.assertIn('query', config)
                self.assertIn('jqfilter', config)
                bigquery.validate_metric_name(config['metric'].strip())
        configs = bigquery.all_configs()
        self.assertTrue(all(p.endswith('.yaml') for p in configs), configs)


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
    for i in xrange(len(sys.argv)):
        if sys.argv[i].startswith('--jq'):
            arg = sys.argv.pop(i)
            if '=' not in arg:
                del sys.argv[i]
            break

    unittest.main()
