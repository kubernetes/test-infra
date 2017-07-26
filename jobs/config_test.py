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


import json
import unittest

import config_sort


class JobTest(unittest.TestCase):

    def test_config_is_sorted(self):
        """Test jobs/config.json, prow/config.yaml and boskos/resources.json are sorted."""
        with open(config_sort.test_infra('jobs/config.json')) as fp:
            original = fp.read()
            expect = json.dumps(
                json.loads(original),
                sort_keys=True,
                indent=2,
                separators=(',', ': ')
                ) + '\n'
            if original != expect:
                self.fail('jobs/config.json is not sorted, please run '
                          '`bazel run //jobs:config_sort`')
        with open(config_sort.test_infra('prow/config.yaml')) as fp:
            original = fp.read()
            expect = config_sort.sorted_prow_config().getvalue()
            if original != expect:
                self.fail('prow/config.yaml is not sorted, please run '
                          '`bazel run //jobs:config_sort`')
        with open(config_sort.test_infra('boskos/resources.json')) as fp:
            original = fp.read()
            expect = config_sort.sorted_boskos_config().getvalue()
            if original != expect:
                self.fail('boskos/resources.json is not sorted, please run '
                          '`bazel run //jobs:config_sort`')


if __name__ == '__main__':
    unittest.main()
