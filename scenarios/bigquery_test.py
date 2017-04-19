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
import unittest
import tempfile
import shutil
import bigquery

class TestBigquery(unittest.TestCase):
    """Tests the bigquery scenario."""
    out_filename = "out.json"
    data_filename = "data.json"
    sample_sql = "#standardSQL\n select job from `k8s-gubernator.build.all`"

    def test_query(self):
        """Test that the do query function can execute a query without failing."""
        tests = [[self.sample_sql, self.out_filename, "k8s-gubernator"]]
        for test in tests:
            bigquery.do_query(*test)
            with open(self.out_filename) as out_file:
                contents = out_file.read()
                if "error" in contents.split("\n")[0].lower():
                    raise ValueError("query should have thrown an error because the"
                                     " bq command failed.")

    def test_jq(self):
        """Test that the do_jq function can execute a jq filter properly."""
        # [filter, data, expected output]
        tests = [[".", '{ "field": "value" }', '{"field":"value"}'],
                 [".field", '{ "field": "value" }', '"value"']]
        for test in tests:
            with open(self.data_filename, "w") as data_file:
                data_file.write(test[1])
            bigquery.do_jq(test[0], self.data_filename, self.out_filename)
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
        self.tmpdir = tempfile.mkdtemp(prefix="bigquery_test_")
        os.chdir(self.tmpdir)

    def tearDown(self):
        shutil.rmtree(self.tmpdir)

if __name__ == "__main__":
    unittest.main()
