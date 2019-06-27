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

"""Generate JSON for BigQuery importing."""

import argparse
import logging
import json
import os
import subprocess
import sys
import time
import traceback

try:
    import defusedxml.ElementTree as ET
except ImportError:
    import xml.etree.cElementTree as ET

import model


def parse_junit(xml):
    """Generate failed tests as a series of dicts. Ignore skipped tests."""
    # NOTE: this is modified from gubernator/view_build.py
    tree = ET.fromstring(xml)

    # pylint: disable=redefined-outer-name

    def make_result(name, time, failure_text):
        if failure_text:
            if time is None:
                return {'name': name, 'failed': True, 'failure_text': failure_text}
            return {'name': name, 'time': time, 'failed': True, 'failure_text': failure_text}
        if time is None:
            return {'name': name}
        return {'name': name, 'time': time}

    # Note: skipped tests are ignored because they make rows too large for BigQuery.
    # Knowing that a given build could have ran a test but didn't for some reason
    # isn't very interesting.
    if tree.tag == 'testsuite':
        for child in tree.findall('testcase'):
            name = child.attrib['name']
            time = float(child.attrib['time'] or 0)
            failure_text = None
            for param in child.findall('failure'):
                failure_text = param.text
            skipped = child.findall('skipped')
            if skipped:
                continue
            yield make_result(name, time, failure_text)
    elif tree.tag == 'testsuites':
        for testsuite in tree:
            suite_name = testsuite.attrib['name']
            for child in testsuite.findall('testcase'):
                name = '%s %s' % (suite_name, child.attrib['name'])
                time = float(child.attrib['time'] or 0)
                failure_text = None
                for param in child.findall('failure'):
                    failure_text = param.text
                skipped = child.findall('skipped')
                if skipped:
                    continue
                yield make_result(name, time, failure_text)
    else:
        logging.error('unable to find failures, unexpected tag %s', tree.tag)


def buckets_yaml():
    import yaml  # does not support pypy
    with open(os.path.dirname(os.path.abspath(__file__))+'/buckets.yaml') as fp:
        return yaml.load(fp)

# pypy compatibility hack
def python_buckets_yaml(python='python2'):
    return json.loads(subprocess.check_output(
        [python, '-c', 'import json,yaml; print json.dumps(yaml.load(open("buckets.yaml")))'],
        cwd=os.path.dirname(os.path.abspath(__file__))))

for attempt in [python_buckets_yaml, buckets_yaml, lambda: python_buckets_yaml(python='python')]:
    try:
        BUCKETS = attempt()
        break
    except (ImportError, OSError):
        traceback.print_exc()
else:
    # pylint: disable=misplaced-bare-raise
    # This is safe because the only way we get here is by faling all attempts
    raise


def path_to_job_and_number(path):
    assert not path.endswith('/')
    for bucket, meta in BUCKETS.iteritems():
        if path.startswith(bucket):
            prefix = meta['prefix']
            break
    else:
        if path.startswith('gs://kubernetes-jenkins/pr-logs'):
            prefix = 'pr:'
        else:
            raise ValueError('unknown build path')
    build = os.path.basename(path)
    job = prefix + os.path.basename(os.path.dirname(path))
    try:
        return job, int(build)
    except ValueError:
        return job, None


def row_for_build(path, started, finished, results):
    tests = []
    for result in results:
        for test in parse_junit(result):
            if '#' in test['name'] and not test.get('failed'):
                continue  # skip successful repeated tests
            tests.append(test)
    build = {
        'path': path,
        'test': tests,
        'tests_run': len(tests),
        'tests_failed': sum(t.get('failed', 0) for t in tests)
    }
    job, number = path_to_job_and_number(path)
    build['job'] = job
    if number:
        build['number'] = number

    if started:
        build['started'] = int(started['timestamp'])
        if 'node' in started:
            build['executor'] = started['node']
    if finished:
        build['finished'] = int(finished['timestamp'])
        if 'result' in finished:
            build['result'] = finished['result']
            build['passed'] = build['result'] == 'SUCCESS'
        elif isinstance(finished.get('passed'), bool):
            build['passed'] = finished['passed']
            build['result'] = 'SUCCESS' if build['passed'] else 'FAILURE'
        if 'version' in finished:
            build['version'] = finished['version']

    def get_metadata():
        metadata = None
        if finished and 'metadata' in finished:
            metadata = finished['metadata']
        elif started:
            metadata = started.get('metadata')
        if metadata:
            # clean useless/duplicated metadata fields
            if 'repo' in metadata and not metadata['repo']:
                metadata.pop('repo')
            build_version = build.get('version', 'N/A')
            if metadata.get('job-version') == build_version:
                metadata.pop('job-version')
            if metadata.get('version') == build_version:
                metadata.pop('version')
            for key, value in metadata.items():
                if not isinstance(value, basestring):
                    # the schema specifies a string value. force it!
                    metadata[key] = json.dumps(value)
        if not metadata:
            return None
        return [{'key': k, 'value': v} for k, v in sorted(metadata.items())]

    metadata = get_metadata()
    if metadata:
        build['metadata'] = metadata
    if started and finished:
        build['elapsed'] = build['finished'] - build['started']
    return build


def get_table(days):
    if days:
        return ('build_emitted_%g' % days).replace('.', '_')
    return 'build_emitted'


def parse_args(args):
    parser = argparse.ArgumentParser()
    parser.add_argument('--days', type=float, default=0,
                        help='Grab data for builds within N days')
    parser.add_argument('--assert-oldest', type=float,
                        help='Exit nonzero if a build older than X days was emitted previously.')
    parser.add_argument('--reset-emitted', action='store_true',
                        help='Clear list of already-emitted builds.')
    parser.add_argument('paths', nargs='*',
                        help='Options list of gs:// paths to dump rows for.')
    return parser.parse_args(args)


def make_rows(db, builds):
    for rowid, path, started, finished in builds:
        try:
            results = db.test_results_for_build(path)
            yield rowid, row_for_build(path, started, finished, results)
        except IOError:
            return
        except:  # pylint: disable=bare-except
            logging.exception('error on %s', path)


def main(db, opts, outfile):
    min_started = None
    if opts.days:
        min_started = time.time() - (opts.days or 1) * 24 * 60 * 60
    incremental_table = get_table(opts.days)

    if opts.assert_oldest:
        oldest = db.get_oldest_emitted(incremental_table)
        if oldest < time.time() - opts.assert_oldest * 24 * 60 * 60:
            return 1
        return 0

    if opts.reset_emitted:
        db.reset_emitted(incremental_table)

    if opts.paths:
        # When asking for rows for specific builds, use a dummy table and clear it first.
        incremental_table = 'incremental_manual'
        db.reset_emitted(incremental_table)
        builds = list(db.get_builds_from_paths(opts.paths, incremental_table))
    else:
        builds = db.get_builds(min_started=min_started, incremental_table=incremental_table)

    rows_emitted = set()
    for rowid, row in make_rows(db, builds):
        json.dump(row, outfile, sort_keys=True)
        outfile.write('\n')
        rows_emitted.add(rowid)

    if rows_emitted:
        gen = db.insert_emitted(rows_emitted, incremental_table=incremental_table)
        print >>sys.stderr, 'incremental progress gen #%d' % gen
    else:
        print >>sys.stderr, 'no rows emitted'
    return 0


if __name__ == '__main__':
    DB = model.Database()
    OPTIONS = parse_args(sys.argv[1:])
    sys.exit(main(DB, OPTIONS, sys.stdout))
