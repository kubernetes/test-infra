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

import argparse
import hashlib
import logging
import json
import os
import sqlite3
import subprocess
import sys
import time

try:
    import defusedxml.ElementTree as ET
except ImportError:
    import xml.etree.cElementTree as ET

import model


def parse_junit(xml):
    """Generate failed tests as a series of dicts. Ignore skipped tests."""
    # NOTE: this is modified from gubernator/view_build.py
    tree = ET.fromstring(xml)

    def make_result(name, time, failure_text, skipped):
        if failure_text:
            if time is None:
                return {'name': name, 'failed': True, 'failure_text': failure_text}
            else:
                return {'name': name, 'time': time, 'failed': True, 'failure_text': failure_text}
        else:
            if time is None:
                return {'name': name}
            else:
                return {'name': name, 'time': time}

    # Note: skipped tests are ignored because they make rows too large for BigQuery.
    # Knowing that a given build could have ran a test but didn't for some reason isn't very interesting.
    if tree.tag == 'testsuite':
        for child in tree:
            name = child.attrib['name']
            time = float(child.attrib['time'] or 0)
            failure_text = None
            for param in child.findall('failure'):
                failure_text = param.text
            skipped = child.findall('skipped')
            if skipped:
                continue
            yield make_result(name, time, failure_text, skipped)
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
                yield make_result(name, time, failure_text, skipped)
    else:
        logging.error('unable to find failures, unexpected tag %s', tree.tag)


# pypy compatibility hack
BUCKETS = json.loads(subprocess.check_output(
    ['python', '-c', 'import json,yaml; print json.dumps(yaml.load(open("../buckets.yaml")))'],
    cwd=os.path.dirname(__file__)))


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
        tests.extend(parse_junit(result))
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
        if 'jenkins-node' in started:
            build['executor'] = started['jenkins-node']
        if 'node' in started:
            build['executor'] = started['node']
    if finished:
        build['finished'] = int(finished['timestamp'])
        build['result'] = finished['result']
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
            for k, v in metadata.items():
                if not isinstance(v, basestring):
                    # the schema specifies a string value. force it!
                    metadata[k] = json.dumps(v)
        if not metadata:
            return None
        return [{'key': k, 'value': v} for k, v in sorted(metadata.items())]

    metadata = get_metadata()
    if metadata:
        build['metadata'] = metadata
    if started and finished:
        build['elapsed'] = build['finished'] - build['started']
    return build


def parse_args(args):
    parser = argparse.ArgumentParser()
    parser.add_argument('--day', action='store_true',
                        help='Grab data for builds in the last day')
    parser.add_argument('--days', type=float,
                        help='Grab data for builds within N days')
    parser.add_argument('--reset-emitted', action='store_true',
                        help='Clear list of already-emitted builds.')
    return parser.parse_args(args)

def main(db, opts, outfile):
    incremental_table = 'build_emitted'

    min_started = None
    if opts.day or opts.days:
        min_started = time.time() - (opts.days or 1) * 24 * 60 * 60
        incremental_table = ('build_emitted_%g' % (opts.days or 1)).replace('.', '_')

    if opts.reset_emitted:
        db.reset_emitted(incremental_table)

    builds = db.get_builds(min_started=min_started, incremental_table=incremental_table)

    rows_emitted = set()
    for rowid, path, started, finished in builds:
        try:
            results = db.test_results_for_build(path)
            row = row_for_build(path, started, finished, results)
            json.dump(row, outfile, sort_keys=True)
            outfile.write('\n')
            rows_emitted.add(rowid)
        except IOError:
            return
        except:
            logging.exception('error on %s', path)

    if rows_emitted:
        gen = db.insert_emitted(rows_emitted, incremental_table=incremental_table)
        print >>sys.stderr, 'incremental progress gen #%d' % gen
    else:
        print >>sys.stderr, 'no rows emitted'


if __name__ == '__main__':
    db = model.Database('build.db')
    opts = parse_args(sys.argv[1:])
    main(db, opts, sys.stdout)
