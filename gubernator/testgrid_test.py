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

import unittest

import testgrid

import main_test
import pb_glance_test
import gcs_async_test


def write_config():
    tab = 'ajob'
    path = 'kubernetes-jenkins/logs/somejob'
    tab_len = 2 + len(tab) + 2 + 2
    data = pb_glance_test.tostr(
        [
            (1<<3)|2, 2 + 2 + 2 + len(path),  # testgroup
            (1<<3)|2, 2, 'tg',                # testgroup.name
            (2<<3)|2, len(path), path,        # testgroup.query
            (2<<3)|2, 2 + 3 + 2 + tab_len,        # dashboard
            (2<<3)|2, 3, 'k8s',               # dashboard.name
            (1<<3)|2, tab_len,                # dashboard.tab
            (1<<3)|2, len(tab), tab,          # dashboard.tab.name
            (2<<3)|2, len('tg'), 'tg',        # dashboard.tab.test_group_name
        ]
    )
    gcs_async_test.write('/k8s-testgrid/config', data)


class TestTestgrid(unittest.TestCase):
    # pylint: disable=protected-access

    def test_path_to_group(self):
        testgrid._testgrid_config = {
            'test_groups': [
                {'query': []},
                {'query': ['foo']},
                {'query': ['bar/e'], 'name': ['blah']},
            ]
        }
        self.assertEqual(testgrid.path_to_group_name('bar/e'), 'blah')
        self.assertEqual(testgrid.path_to_group_name('/blah'), None)

    def test_path_to_query(self):
        testgrid._testgrid_config = {
            'test_groups': [
                {'query': ['gce-serial'], 'name': ['gce-serial']},
                {'query': ['gce-soak'], 'name': ['gce-soak']},
                {'query': ['gce-soak-1.3'], 'name': ['gce-soak-1.3']},
                {'query': ['gke'], 'name': ['gke']},
                {'query': ['unusedgroup'], 'name': ['unused']},
                {'query': ['pr-logs/directory/pull-gce'], 'name': ['pull-gce']},
                {'query': ['pr-logs/directory/pull-ti-verify'], 'name': ['pull-ti-verify']},
            ],
            'dashboards': [
                {
                    'name': ['k8s'],
                    'dashboard_tab': [
                        {'test_group_name': ['gce-serial'], 'name': ['serial']},
                    ]
                },
                {
                    'name': ['gce'],
                    'dashboard_tab': [
                        {'test_group_name': ['gce-serial'], 'name': ['serial']},
                        {'test_group_name': ['gce-soak'], 'name': ['soak']},
                        {'test_group_name': ['gce-soak-1.3'], 'name': ['soak-1.3']},
                    ]
                },
                {
                    'name': ['1.3'],
                    'dashboard_tab': [
                        {'test_group_name': ['gce-soak-1.3'], 'name': ['soak']},
                    ]
                },
                {
                    'name': ['gke'],
                    'dashboard_tab': [
                       {'test_group_name': ['gke'], 'name': ['gke']}
                    ]
                },
                {
                    'name': ['sig-storage'],
                    'dashboard_tab': [
                       {'test_group_name': ['gke'], 'name': ['gke'],
                         'base_options': ['include-filter-by-regex=Storage']},
                       {'test_group_name': ['gce-serial'], 'name': ['gce-serial'],
                         'base_options': ['include-filter-by-regex=Storage']}
                    ]
                },
                {
                    'name': ['pull'],
                    'dashboard_tab': [
                       {'test_group_name': ['pull-gce'], 'name': ['gce'],
                        'base_options': ['width=10']},
                    ]
                },
                {
                    'name': ['pull-ti'],
                    'dashboard_tab': [
                       {'test_group_name': ['pull-ti-verify'], 'name': ['verify']},
                    ]
                },
            ]
        }
        def expect(path, out):
            self.assertEqual(testgrid.path_to_query(path), out)

        expect('/gce-serial/', 'k8s#serial')
        expect('gce-soak', 'gce#soak')
        expect('gce-soak-1.3', 'gce#soak-1.3')
        expect('unusedgroup', '')
        expect('notarealpath', '')
        expect('gke', 'gke#gke')
        expect('pr-logs/pull/123/pull-gce/', 'pull#gce')
        expect('pr-logs/pull/ti/123/pull-ti-verify/', 'pull-ti#verify')


class TestTestgridGCS(main_test.TestBase):
    def test_write_config(self):
        # pylint: disable=protected-access
        self.init_stubs()
        testgrid._testgrid_config = None
        write_config()
        path = 'kubernetes-jenkins/logs/somejob'
        self.assertEqual(
            testgrid.get_config(),
            {
                'test_groups': [{
                    'name': ['tg'],
                    'query': [path]
                }],
                'dashboards': [{
                    'dashboard_tab': [{'name': ['ajob'], 'test_group_name': ['tg']}],
                    'name': ['k8s']
                }],
            })
        self.assertEqual(testgrid.path_to_query(path), 'k8s#ajob')
