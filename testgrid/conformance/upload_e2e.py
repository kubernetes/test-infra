#!/usr/bin/env python3

# Copyright 2018 The Kubernetes Authors.
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

# This script parses conformance test output to produce testgrid entries
#
# Assumptions:
# - there is one log file and one JUnit file (true for current conformance tests..)
# - the log file contains ginkgo's output (true for kubetest and sonobuoy..)
# - the ginkgo output will give us start / end time, and overall success
#
# - the start timestamp is suitable as a testgrid ID (unique, monotonic)
#
# - the test ran in the current year unless --year is provided
# - the timestamps are parsed on a machine with the same local time (zone)
#   settings as the machine that produced the logs
#
# The log file is the source of truth for metadata, the JUnit will be consumed
# by testgrid / gubernator for individual test case results
#
# Usage: see README.md


import re
import sys
import time
import datetime
import argparse
import json
import subprocess
from os import path
import glob
import atexit


# logs often contain ANSI escape sequences
# https://stackoverflow.com/a/14693789
ANSI_ESCAPE_RE = re.compile(r'\x1B\[[0-?]*[ -/]*[@-~]')


# NOTE e2e logs use go's time.StampMilli ("Jan _2 15:04:05.000")
# Example log line with a timestamp:
# Jan 26 06:38:46.284: INFO: Running AfterSuite actions on all node
# the third ':' separates the date from the rest
E2E_LOG_TIMESTAMP_RE = re.compile(r'(... .\d \d\d:\d\d:\d\d\.\d\d\d):.*')

# Ginkgo gives a line like the following at the end of successful runs:
# SUCCESS! -- 123 Passed | 0 Failed | 0 Pending | 587 Skipped PASS
# we match this to detect overall success
E2E_LOG_SUCCESS_RE = re.compile(r'Test Suite Passed')
E2E_LOG_FAIL_RE = re.compile(r'Test Suite Failed')


def log_line_strip_escape_sequences(line):
    return ANSI_ESCAPE_RE.sub('', line)


def parse_e2e_log_line_timestamp(line, year):
    """parses a ginkgo e2e log line for the leading timestamp

    Args:
        line (str) - the log line
        year (str) - 'YYYY'

    Returns:
        timestamp (datetime.datetime) or None
    """
    match = E2E_LOG_TIMESTAMP_RE.match(line)
    if match is None:
        return None
    # note we add year to the timestamp because the actual timestamp doesn't
    # contain one and we want a datetime object...
    timestamp = year+' '+match.group(1)
    return datetime.datetime.strptime(timestamp, '%Y %b %d %H:%M:%S.%f')


def parse_e2e_logfile(file_handle, year):
    """parse e2e logfile at path, assuming the log is from year

    Args:
        file_handle (file): the log file, iterated for lines
        year (str): YYYY year logfile is from

    Returns:
        started (datetime.datetime), finished (datetime.datetime), passed (boolean)
    """
    passed = started = finished = None
    for line in file_handle:
        line = log_line_strip_escape_sequences(line)
        # try to get a timestamp from each line, keep the first one as
        # start time, and the last one as finish time
        timestamp = parse_e2e_log_line_timestamp(line, year)
        if timestamp:
            if started:
                finished = timestamp
            else:
                started = timestamp
        if passed is False:
            # if we already have found a failure, ignore subsequent pass/fails
            continue
        if E2E_LOG_SUCCESS_RE.match(line):
            passed = True
        elif E2E_LOG_FAIL_RE.match(line):
            passed = False
    return started, finished, passed


def datetime_to_unix(datetime_obj):
    """convert datetime.datetime to unix timestamp"""
    return int(time.mktime(datetime_obj.timetuple()))


def testgrid_started_json_contents(start_time):
    """returns the string contents of a testgrid started.json file

    Args:
        start_time (datetime.datetime)

    Returns:
        contents (str)
    """
    started = datetime_to_unix(start_time)
    return json.dumps({
        'timestamp': started
    })


def testgrid_finished_json_contents(finish_time, passed, metadata):
    """returns the string contents of a testgrid finished.json file

    Args:
        finish_time (datetime.datetime)
        passed (bool)
        metadata (str)

    Returns:
        contents (str)
    """
    finished = datetime_to_unix(finish_time)
    result = 'SUCCESS' if passed else 'FAILURE'
    if metadata:
        testdata = json.loads(metadata)
        return json.dumps({
            'timestamp': finished,
            'result': result,
            'metadata': testdata
        })
    return json.dumps({
        'timestamp': finished,
        'result': result
    })


def upload_string(gcs_path, text, dry):
    """Uploads text to gcs_path if dry is False, otherwise just prints"""
    cmd = ['gsutil', '-q', '-h', 'Content-Type:text/plain', 'cp', '-', gcs_path]
    print('Run:', cmd, 'stdin=%s' % text, file=sys.stderr)
    if dry:
        return
    proc = subprocess.Popen(cmd, stdin=subprocess.PIPE, encoding='utf8')
    proc.communicate(input=text)
    if proc.returncode != 0:
        raise RuntimeError(
            "Failed to upload with exit code: %d" % proc.returncode)


def upload_file(gcs_path, file_path, dry):
    """Uploads file at file_path to gcs_path if dry is False, otherwise just prints"""
    cmd = ['gsutil', '-q', '-h', 'Content-Type:text/plain',
           'cp', file_path, gcs_path]
    print('Run:', cmd, file=sys.stderr)
    if dry:
        return
    proc = subprocess.Popen(cmd)
    proc.communicate()
    if proc.returncode != 0:
        raise RuntimeError(
            'Failed to upload with exit code: %d' % proc.returncode)


def get_current_account(dry_run):
    """gets the currently active gcp account by shelling out to gcloud"""
    cmd = ['gcloud', 'auth', 'list',
           '--filter=status:ACTIVE', '--format=value(account)']
    print('Run:', cmd, file=sys.stderr)
    if dry_run:
        return ""
    return subprocess.check_output(cmd, encoding='utf-8').strip('\n')


def set_current_account(account, dry_run):
    """sets the currently active gcp account by shelling out to gcloud"""
    cmd = ['gcloud', 'config', 'set', 'core/account', account]
    print('Run:', cmd, file=sys.stderr)
    if dry_run:
        return None
    return subprocess.check_call(cmd)


def activate_service_account(key_file, dry_run):
    """activates a gcp service account by shelling out to gcloud"""
    cmd = ['gcloud', 'auth', 'activate-service-account', '--key-file='+key_file]
    print('Run:', cmd, file=sys.stderr)
    if dry_run:
        return
    subprocess.check_call(cmd)


def revoke_current_account(dry_run):
    """logs out of the currently active gcp account by shelling out to gcloud"""
    cmd = ['gcloud', 'auth', 'revoke']
    print('Run:', cmd, file=sys.stderr)
    if dry_run:
        return None
    return subprocess.check_call(cmd)


def parse_args(cli_args=None):
    if cli_args is None:
        cli_args = sys.argv[1:]
    parser = argparse.ArgumentParser()
    parser.add_argument(
        '--bucket',
        help=('GCS bucket to upload the results to,'
              ' of the form \'gs://foo/bar\''),
        required=True,
    )
    parser.add_argument(
        '--year',
        help=('the year in which the log is from, defaults to the current year.'
              ' format: YYYY'),
        default=str(datetime.datetime.now().year),
    )
    parser.add_argument(
        '--junit',
        help='path or glob expression to the junit xml results file(s)',
        required=True,
    )
    parser.add_argument(
        '--log',
        help='path to the test log file, should contain the ginkgo output',
        required=True,
    )
    parser.add_argument(
        '--dry-run',
        help='if set, do not actually upload anything, only print actions',
        required=False,
        action='store_true',
    )
    parser.add_argument(
        '--metadata',
        help='dictionary of additional key-value pairs that can be displayed to the user.',
        required=False,
        default=str(),
    )
    parser.add_argument(
        '--key-file',
        help='path to GCP service account key file, which will be activated before '
        'uploading if provided, the account will be revoked and the active account reset '
        'on exit',
        required=False,
    )
    return parser.parse_args(args=cli_args)


def main(cli_args):
    args = parse_args(cli_args)

    # optionally activate a service account with upload credentials
    if args.key_file:
        # grab the currently active account if any, and if there is one
        # register a handler to set it active again on exit
        current_account = get_current_account(args.dry_run)
        if current_account:
            atexit.register(
                lambda: set_current_account(current_account, args.dry_run)
            )
        # login to the service account and register a handler to logout before exit
        # NOTE: atexit handlers are called in LIFO order
        activate_service_account(args.key_file, args.dry_run)
        atexit.register(lambda: revoke_current_account(args.dry_run))

    # find the matching junit files, there should be at least one for a useful
    # testgrid entry
    junits = glob.glob(args.junit)
    if not junits:
        print('No matching JUnit files found!')
        sys.exit(-1)

    # parse the e2e.log for start time, finish time, and success
    with open(args.log) as file_handle:
        started, finished, passed = parse_e2e_logfile(file_handle, args.year)

    # convert parsed results to testgrid json metadata blobs
    started_json = testgrid_started_json_contents(started)
    finished_json = testgrid_finished_json_contents(
        finished, passed, args.metadata)

    # use timestamp as build ID
    gcs_dir = args.bucket + '/' + str(datetime_to_unix(started))

    # upload metadata, log, junit to testgrid
    print('Uploading entry to: %s' % gcs_dir)
    upload_string(gcs_dir+'/started.json', started_json, args.dry_run)
    upload_string(gcs_dir+'/finished.json', finished_json, args.dry_run)
    upload_file(gcs_dir+'/build-log.txt', args.log, args.dry_run)
    for junit_file in junits:
        upload_file(gcs_dir+'/artifacts/' +
                    path.basename(junit_file), junit_file, args.dry_run)
    print('Done.')


if __name__ == '__main__':
    main(sys.argv[1:])
