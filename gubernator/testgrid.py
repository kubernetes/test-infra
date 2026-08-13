#!/usr/bin/env python
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

import logging
import re

import cloudstorage as gcs

import pb_glance


CONFIG_PROTO_SCHEMA = {
    1: {
        'name': 'test_groups',
        1: 'name',
        2: 'query',
        9: {},
    },
    2: {
        'name': 'dashboards',
        1: {
            'name': 'dashboard_tab',
            1: 'name',
            2: 'test_group_name',
            6: 'base_options',
            7: {},
            8: {2: {}},
            9: {},
            11: {},
            12: {},
        },
        2: 'name',
    }
}

_testgrid_config = None


def get_config():
    """
    Load the testgrid config loaded from a proto stored on GCS.
    It will be cached locally in memory for the life of this process.

    Returns:
        dict: {
            'test_groups': [{'name': ..., 'query': ...}],
            'dashboards': [{
                'name': ...,
                'dashboard_tab': [{'name': ..., 'test_group_name': ...}]
            }]
        }
    """
    global _testgrid_config  # pylint: disable=global-statement
    if not _testgrid_config:
        try:
            data = gcs.open('/k8s-testgrid/config').read()
        except gcs.NotFoundError:
            # Fallback to local files for development-- the k8s-testgrid bucket
            # has restrictive ACLs that dev_appserver.py can't read.
            data = open('tg-config').read()
        _testgrid_config = pb_glance.parse_protobuf(data, CONFIG_PROTO_SCHEMA)
    return _testgrid_config


def path_to_group_name(path):
    """
    Args:
        path: a job directory like "/kubernetes-jenkins/jobs/e2e-gce"
    Returns:
        test_group_name: the group name in the config, or None if not found
    """
    try:
        config = get_config()
    except gcs.errors.Error:
        logging.exception('unable to load testgrid config')
        return None
    path = path.strip('/')  # the config doesn't have leading/trailing slashes
    if '/pull/' in path:  # translate PR to all-pr result form
        path = re.sub(r'/pull/([^/]+/)?\d+/', '/directory/', path)
    for test_group in config.get('test_groups', []):
        if path in test_group['query']:
            return test_group['name'][0]


def path_to_query(path):
    """
    Convert a GCS job directory to the testgrid path for its results.

    Args:
        path: a job directory like "/kubernetes-jenkins/jobs/e2e-gce"
    Returns:
        query: the url for the job, like "k8s#gce", or "" if not found.
    """
    group = path_to_group_name(path)
    if not group:
        return ''

    # Tabs can appear on multiple dashboards. Favor selecting 'k8s' over others,
    # otherwise pick a random tab.

    options = {}
    for dashboard in get_config().get('dashboards', []):
        dashboard_name = dashboard['name'][0]
        tabs = dashboard['dashboard_tab']
        for (skip_base_options, penalty) in ((True, 0), (False, 1000)):
            for tab in tabs:
                if 'base_options' in tab and skip_base_options:
                    continue
                if group in tab['test_group_name']:
                    query = '%s#%s' % (dashboard_name, tab['name'][0])
                    options[dashboard_name] = (-len(tabs) + penalty, query)
            if dashboard_name in options:
                break
    if 'k8s' in options:
        return options['k8s'][1]
    elif len(options) > 1:
        logging.info('ambiguous testgrid options: %s', options)
    elif len(options) == 0:
        return ''
    return sorted(options.values())[0][1]
