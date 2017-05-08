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

import os
import argparse
import unittest
import tempfile
import shutil
import bigquery

class TestBigquery(unittest.TestCase):
    """Tests the bigquery scenario."""

    def test_jq(self):
        """Test that the do_jq function can execute a jq filter properly."""
        # [filter, data, expected output]
        tests = [[".", '{ "field": "value" }', '{"field":"value"}'],
                 [".field", '{ "field": "value" }', '"value"']]
        for test in tests:
            with open(self.data_filename, "w") as data_file:
                data_file.write(test[1])
            bigquery.do_jq(test[0], self.data_filename, self.out_filename, jq_bin=ARGS.jq)
            with open(self.out_filename) as out_file:
                actual = out_file.read().replace(" ", "").replace("\n", "")
                self.assertEqual(actual, test[2], msg="expected jq '{}' on data: {} to output {}"
                                 " but got {}".format(test[0], test[1], test[2], actual))

    def test_validate_metric_name(self):
        """Test the the validate_metric_name function rejects invalid metric names."""
        tests = ["invalid#metric", "invalid/metric", "in\\valid", "invalid?yes", "*invalid",
                 "[metric]", "metric\n", "met\ric"]
        for test in tests:
            self.assertRaises(ValueError, bigquery.validate_metric_name, test)

    def setUp(self):
        self.assertTrue(ARGS.jq)
        self.tmpdir = tempfile.mkdtemp(prefix="bigquery_test_")
        self.out_filename = os.path.join(self.tmpdir, "out.json")
        self.data_filename = os.path.join(self.tmpdir, "data.json")

    def tearDown(self):
        shutil.rmtree(self.tmpdir)

if __name__ == "__main__":
    PARSER = argparse.ArgumentParser()
    PARSER.add_argument("--jq",
                        required=True,
                        type=str,
                        help="The path to the 'jq' command.")
    ARGS = PARSER.parse_args()

    RESULTS = unittest.TextTestRunner().run(
        unittest.TestLoader().loadTestsFromTestCase(TestBigquery))
    if len(RESULTS.errors) > 0:
        raise ValueError(RESULTS.errors)
