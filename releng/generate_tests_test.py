#!/usr/bin/env python3

# Copyright 2019 The Kubernetes Authors.
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

import unittest
import tempfile
import shutil
from generate_tests import E2ETest

class TestGenerateTests(unittest.TestCase):

    def setUp(self):
        self.temp_directory = tempfile.mkdtemp()
        self.job_name = "ci-kubernetes-e2e-gce-cos-k8sbeta-ingress"
        self.job = {
            "interval": "1h"
        }
        self.config = {
            "jobs": {"ci-kubernetes-e2e-gce-cos-k8sbeta-ingress": self.job},
            "common": {"args": []},
            "cloudProviders": {"gce": {"args": []}},
            "images": {"cos": {}},
            "k8sVersions": {"beta": {"version": "2.4"}},
            "testSuites": {"ingress": {"args": ["--timeout=10"]}},
        }

    def tearDown(self):
        shutil.rmtree(self.temp_directory)

    def test_e2etests_testgrid_annotations_default(self):
        generator = E2ETest(self.temp_directory, self.job_name, self.job, self.config)
        _, prow_config, _ = generator.generate()
        dashboards = prow_config["annotations"]["testgrid-dashboards"]
        self.assertFalse("sig-release-2.4-blocking" in dashboards)
        self.assertTrue("sig-release-2.4-all" in dashboards)

    def test_e2etests_testgrid_annotations_blocking_job(self):
        self.job = {
            "releaseBlocking": True,
            "interval": "1h"
        }

        generator = E2ETest(self.temp_directory, self.job_name, self.job, self.config)
        _, prow_config, _ = generator.generate()
        dashboards = prow_config["annotations"]["testgrid-dashboards"]
        self.assertTrue("sig-release-2.4-blocking" in dashboards)
        self.assertFalse("sig-release-2.4-all" in dashboards)


if __name__ == '__main__':
    unittest.main()
