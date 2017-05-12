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
        start = os.path.join(os.path.dirname(__file__), '../metrics')
        for root, _, names in os.walk(start):
            for name in names:
                if name in ['BUILD', 'README.md']:
                    continue
                if not name.endswith('.yaml'):
                    self.fail('Only .yaml files allowed: %s' % name)

                path = os.path.join(root, name)
                with open(path) as config_file:
                    config = yaml.safe_load(config_file)
                    if not config:
                        self.fail(path)
                    self.assertIn('metric', config)
                    self.assertIn('query', config)
                    self.assertIn('jqfilter', config)
                    bigquery.validate_metric_name(config['metric'].strip())

    def test_jq(self):
        """do_jq executes a jq filter properly."""
        # [filter, data, expected output]
        tests = [
            ['.', '{ "field": "value" }', '{"field":"value"}'],
            ['.field', '{ "field": "value" }', '"value"'],
        ]
        for test in tests:
            with open(self.data_filename, 'w') as data_file:
                data_file.write(test[1])
            bigquery.do_jq(
                test[0],
                self.data_filename,
                self.out_filename,
                jq_bin=ARGS.jq,
            )
            with open(self.out_filename) as out_file:
                actual = out_file.read().replace(' ', '').replace('\n', '')
                self.assertEqual(actual, test[2])

    def test_validate_metric_name(self):
        """validate_metric_name rejects invalid metric names."""
        for test in [
                'invalid#metric',
                'invalid/metric',
                'in\\valid',
                'invalid?yes',
                '*invalid',
                '[metric]',
                'metric\n',
                'met\ric',
                'metric& invalid',
        ]:
            self.assertRaises(ValueError, bigquery.validate_metric_name, test)

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
