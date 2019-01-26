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


def builds_to_table(jobs):
    """
    Convert a list of builds into arguments suitable for rendering.

    Args:
        jobs: a dict of {job: [(build, started.json, finished.json), ...]}
    Returns:
        max_builds: the number of build columns
        headings: a list of [(version, column width, timestamp)]
        rows: a list of [(build, [(number, status) or None])]
    """
    # pylint: disable=too-many-locals

    def commit(started, finished):
        if 'pull' in started:
            return started['pull'].split(':')[-1]
        if 'version' in started:
            return started['version'].split('+')[-1]
        if finished and 'revision' in finished:
            return finished['revision']
        return 'unknown'

    # Compute the headings first -- versions and their maximum build counts.

    versions = {}       # {version: {job: build_count}}
    version_start = {}  # {version: first_build_start_time}
    for job, builds in jobs.iteritems():
        for build, started, finished in builds:
            if not started:
                continue
            version = commit(started, finished)
            if not version:
                continue
            versions.setdefault(version, {}).setdefault(job, 0)
            versions[version][job] += 1
            begin = int(started['timestamp'])
            version_start[version] = min(begin, version_start.get(version, begin))

    version_widths = {version: max(jobs.values()) for version, jobs in versions.iteritems()}
    versions_ordered = sorted(versions, key=lambda v: version_start[v], reverse=True)
    version_colstart = {}
    cur = 0
    for version in versions_ordered:
        version_colstart[version] = cur
        cur += version_widths[version]

    max_builds = cur
    headings = [(version, version_widths[version], version_start[version])
                for version in versions_ordered]

    rows = []
    for job, builds in sorted(jobs.iteritems()):
        row = []
        n = 0
        for build, started, finished in builds:
            if not started or not commit(started, finished):
                minspan = 0
            else:
                minspan = version_colstart[commit(started, finished)]
            while n < minspan:
                row.append(None)
                n += 1
            row.append((build, finished['result'] if finished else 'unfinished'))
            n += 1
        rows.append((job, row))

    return max_builds, headings, rows
