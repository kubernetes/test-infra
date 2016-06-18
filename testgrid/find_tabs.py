#!/usr/bin/env python

# Copyright 2016 The Kubernetes Authors All rights reserved.
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

"""Scan GCS to look for tabs to put into testgrid.

Usage:
  python find_tabs.py [gcs-prefix] [dashboard-prefix]
"""

import collections
import re
import subprocess
import sys

# The value for each of these represents a testgrid dashboard
# group(1) represents the tab name
MATCHES = [
    (r'kubernetes-e2e-gce-(gci.*)', 'gci'),
    (r'kubernetes-e2e-(.*-trusty.*)', 'trusty'),  # contains gce
    (r'kubernetes-e2e-(.*-upgrade.*)', 'upgrade'),  # contains gke
    (r'kubernetes-e2e-(.*kubectl-skew)', 'upgrade'),  # contains gke
    (r'kubernetes-e2e-(gce.*)', 'gce'),
    (r'kubernetes-e2e-(gke.*)', 'gke'),
    (r'kubernetes-e2e-(aws.*)', 'aws'),
    (r'kubernetes-(kubemark-.*)', 'kubemark'),
    (r'kubernetes-(soak.*)', 'soak'),
    (r'(.*-gce-e2e)-ci', 'node'),
    (r'(.*-dockercanarybuild)-ci', 'node'),
    (r'kubernetes-(build.*)', 'unit'),
    (r'kubernetes-(test.*)', 'unit'),
    (r'kubernetes-(verify.*)', 'unit'),
]


def find_tabs(raw_prefix):
    """Yields (dash, tab_name, path) tuples by parsing gsutil ls output."""
    prefix = 'gs://%s' % raw_prefix
    print >>sys.stderr, 'Listing %s...' % prefix
    tabs = subprocess.check_output(['gsutil', 'ls', prefix])
    interesting = re.compile(r'^%s/([^/]+)/' % re.escape(prefix))
    dashboards = [(re.compile(regex), dash) for (regex, dash) in MATCHES]
    for line in tabs.split('\n'):
        mat = interesting.match(line.strip())
        if not mat:  # Ignore weird lines
            continue
        path = mat.group(1)
        for regex, dash in dashboards:
          mat = regex.match(path)
          if not mat:
              continue
          tab_name = mat.group(1)
          yield dash, tab_name, path
          break


def fill_dashboards(tabs):
    """Returns {dash: [(tab, path)]}, set([path]) tuple from found tabs."""
    dashboards = collections.defaultdict(list)
    test_groups = set()
    for (dash, tab, path) in tabs:
        dashboards[dash].append((tab, path))
        test_groups.add(path)
    return dashboards, test_groups


def render_dashboard(category, tabs, prefix):
    """Renders a dashboard config string.

    Follows this format:
      {
        name = 'dashboard_name'
        dashboard_tab = [
          tab('tab-name', 'test-group-name'),
          ...
        ]
      }
    """
    if '\'' in prefix:
      raise ValueError(prefix)
    if '\'' in category:
      raise ValueError(category)
    for tab in tabs:
      if '\'' in tab:
        raise ValueError(tab, tabs)
    return """{
  name = '%(prefix)s-%(category)s'
  dashboard_tab = [
    %(tabs)s
  ]
},""" % dict(
    prefix=prefix,
    category=category,
    tabs='\n    '.join('tab(\'%s\', \'%s\'),' % (tab, path)
                       for (tab, path) in sorted(tabs)))

def render_dashboards(categories, prefix):
    """Renders multiple dashboards, prepending the specified prefix."""
    # Separate each list item with a comma
    return '\n'.join(render_dashboard(c, t, prefix)
                     for (c, t) in sorted(categories.items()))


def render_group(path, prefix):
    """Renders test_groups which represents a set of CI results.

    Follows this format:
      test_group('test-group-name', 'gcs-path')
    """
    return 'test_group(\n        \'%s\',\n        \'%s/%s\'),' % (
        path, prefix, path)


def render_groups(groups, prefix):
    """Renders multiple test_groups, prepending the specified gcs prefix."""
    # Items are comma separated
    return 'test_groups = [\n    %s\n]' % '\n    '.join(
        render_group(g, prefix) for g in sorted(groups))


def configure_testgrid(gcs_prefix='kubernetes-jenkins/logs',
                       dash_prefix='google'):
    """Prints out the test_groups and dashboards list for testgrid."""
    tabs = find_tabs(gcs_prefix)
    dashes, groups = fill_dashboards(tabs)
    print render_groups(groups, gcs_prefix)
    print
    print render_dashboards(dashes, dash_prefix)


if __name__ == '__main__':
    configure_testgrid(*sys.argv[1:])
