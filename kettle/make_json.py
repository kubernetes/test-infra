#!/usr/bin/env python3

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

SECONDS_PER_DAY = 86400

def buckets_yaml():
    import ruamel.yaml as yaml  # pylint: disable=import-outside-toplevel
    with open(os.path.dirname(os.path.abspath(__file__))+'/buckets.yaml') as fp:
        return yaml.safe_load(fp)

# pypy compatibility hack
def python_buckets_yaml(python='python3'):
    return json.loads(subprocess.check_output(
        [python, '-c',
         'import json, ruamel.yaml as yaml; print(json.dumps(yaml.safe_load(open("buckets.yaml"))))'
         ],
        cwd=os.path.dirname(os.path.abspath(__file__))).decode("utf-8"))

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


class Build:
    """
    Represent Metadata and Details of a build. Leveraging the information in
    Started.json and Finished.json
    Should confrom to the schema set in TestGrid below
    github.com/GoogleCloudPlatform/testgrid/blob/7d818/metadata/job.go#L23-L77
    """
    # pylint: disable=too-many-instance-attributes
    # Attrs represent underlying build object

    def __init__(self, path, tests):
        self.path = path
        self.test = tests
        self.tests_run = len(tests)
        self.tests_failed = sum(t.get('failed', 0) for t in tests)
        #From Started.json
        self.started = None
        self.executor = None
        self.repo_commit = None
        #From Finished.json
        self.finished = None
        self.result = None
        self.passed = None
        self.version = None
        #From Either/Combo
        self.repos = None
        self.metadata = None
        self.elapsed = None
        self.populate_path_to_job_and_number()

    @classmethod
    def generate(cls, path, tests, started, finished, metadata, repos):
        build = cls(path, tests)
        build.populate_start(started)
        build.populate_finish(finished)
        build.populate_meta(metadata, repos)
        build.set_elapsed()
        return build

    def populate_path_to_job_and_number(self):
        assert not self.path.endswith('/')
        for bucket, meta in BUCKETS.items():
            if self.path.startswith(bucket):
                prefix = meta['prefix']
                break
        #if job path not in buckets.yaml or gs://kubernetes-jenkins/pr-logs it is unmatched
        else:
            if self.path.startswith('gs://kubernetes-jenkins/pr-logs'):
                prefix = 'pr:'
            else:
                raise ValueError(f'unknown build path for {self.path} in known bucket paths')
        build = os.path.basename(self.path)
        job = prefix + os.path.basename(os.path.dirname(self.path))
        self.job = job
        try:
            self.number = int(build)
        except ValueError:
            self.number = None

    def as_dict(self):
        return {k: v for k, v in self.__dict__.items() if v is not None}

    def populate_start(self, started):
        if started:
            self.started = int(started['timestamp'])
            self.executor = started.get('node')
            self.repo_commit = started.get('repo-commit', started.get('repo-version'))
            self.repos = json.dumps(started.get('repos')) if started.get('repos') else None

    def populate_finish(self, finished):
        if finished:
            self.finished = int(finished['timestamp'])
            self.version = finished.get('version')
            if 'result' in finished:
                self.result = finished.get('result')
                self.passed = self.result == 'SUCCESS'
            elif isinstance(finished.get('passed'), bool):
                self.passed = finished['passed']
                self.result = 'SUCCESS' if self.passed else 'FAILURE'

    def populate_meta(self, metadata, repos):
        self.metadata = metadata
        self.repos = self.repos if self.repos else repos

    def set_elapsed(self):
        if self.started and self.finished:
            self.elapsed = self.finished - self.started


def parse_junit(xml):
    """Generate failed tests as a series of dicts. Ignore skipped tests."""
    # NOTE: this is modified from gubernator/view_build.py
    try:
        tree = ET.fromstring(xml)
    except ET.ParseError:
        print("Malformed xml, skipping")
        yield from [] #return empty itterator to skip results for this test
        return

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

    def parse_result(child_node):
        time = float(child_node.attrib.get('time') or 0) #time val can be ''
        failure_text = None
        for param in child_node.findall('failure'):
            failure_text = param.text or param.attrib.get('message', 'No Failure Message Found')
        skipped = child_node.findall('skipped')
        return time, failure_text, skipped

    try:
        if tree.tag == 'testsuite':
            for child in tree.findall('testcase'):
                name = child.attrib['name']
                time, failure_text, skipped = parse_result(child)
                if skipped:
                    continue
                yield make_result(name, time, failure_text)
        elif tree.tag == 'testsuites':
            for testsuite in tree:
                suite_name = testsuite.attrib['name']
                for child in testsuite.findall('testcase'):
                    name = '%s %s' % (suite_name, child.attrib['name'])
                    time, failure_text, skipped = parse_result(child)
                    if skipped:
                        continue
                    yield make_result(name, time, failure_text)
        else:
            logging.error('unable to find failures, unexpected tag %s', tree.tag)
    except KeyError as err:
        logging.error(f'malformed xml within {tree.tag}: {err}')
        yield from []

def row_for_build(path, started, finished, results):
    """
    Generate an dictionary that represents a build as described by TestGrid's
    job schema. See link for reference.
    github.com/GoogleCloudPlatform/testgrid/blob/7d818/metadata/job.go#L23-L77

    Args:
        path (string): Path to file data for a build
        started (dict): Values pulled from started.json for a build
        finsihed (dict): Values pulled from finsihed.json for a build
        results (array): List of file data that exits under path

    Return:
        Dict holding metadata and information pertinent to a build
        to be stored in BigQuery
    """
    tests = []
    for result in results:
        for test in parse_junit(result):
            if '#' in test['name'] and not test.get('failed'):
                continue  # skip successful repeated tests
            tests.append(test)

    def get_metadata():
        metadata = None
        metapairs = None
        repos = None
        if finished and 'metadata' in finished:
            metadata = finished['metadata']
        elif started:
            metadata = started.get('metadata')

        if metadata:
            # clean useless/duplicated metadata fields
            if 'repo' in metadata and not metadata['repo']:
                metadata.pop('repo')
            build_version = finished.get('version', 'N/A')
            if metadata.get('job-version') == build_version:
                metadata.pop('job-version')
            if metadata.get('version') == build_version:
                metadata.pop('version')
            for key, value in metadata.items():
                if not isinstance(value, str):
                    # the schema specifies a string value. force it!
                    metadata[key] = json.dumps(value)
                    if key == 'repos':
                        repos = metadata[key]
            metapairs = [{'key': k, 'value': v} for k, v in sorted(metadata.items())]
        return metapairs, repos

    metadata, repos = get_metadata()
    build = Build.generate(path, tests, started, finished, metadata, repos)
    return build.as_dict()


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
    min_started = 0
    if opts.days:
        min_started = time.time() - (opts.days or 1) * SECONDS_PER_DAY
    incremental_table = get_table(opts.days)

    if opts.assert_oldest:
        oldest = db.get_oldest_emitted(incremental_table)
        if oldest < time.time() - opts.assert_oldest * SECONDS_PER_DAY:
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
        print('incremental progress gen #%d' % gen, file=sys.stderr)
    else:
        print('no rows emitted', file=sys.stderr)
    return 0


if __name__ == '__main__':
    DB = model.Database()
    OPTIONS = parse_args(sys.argv[1:])
    sys.exit(main(DB, OPTIONS, sys.stdout))
