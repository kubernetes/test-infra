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

"""Runs bigquery metrics and uploads the result to GCS."""

import argparse
import glob
import os
import re
import subprocess
import sys
import time
import traceback

import yaml

def check(*cmd, **keyargs):
    """Logs and runs the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    # If 'stdin' keyword arg is a string run command and communicate string to stdin
    if 'stdin' in keyargs and isinstance(keyargs['stdin'], str):
        in_string = keyargs['stdin']
        keyargs['stdin'] = subprocess.PIPE
        proc = subprocess.Popen(*cmd, **keyargs)
        proc.communicate(input=in_string)
        return
    subprocess.check_call(*cmd, **keyargs)

def validate_metric_name(name):
    """Raise if name is non-trivial."""
    # Regex '$' symbol matches an optional terminating new line
    # so we have to check that the name
    # doesn't have one if the regex matches.
    if not re.match(r'^[\w-]+$', name) or name[-1] == '\n':
        raise ValueError(name)

def do_query(query, out_filename, project):
    """Executes a bigquery query, outputting the results to a file."""
    with open(out_filename, 'w') as out_file:
        cmd = [
            'bq', 'query', '-n100000', '--format=prettyjson', '--project_id=%s' % project]
        check(cmd, stdin=query, stdout=out_file)

def do_jq(jqfilter, data_filename, out_filename, jq_bin='jq'):
    """Executes jq on a data file and outputs the results to a file."""
    with open(out_filename, 'w') as out_file:
        check([jq_bin, jqfilter, data_filename], stdout=out_file)

def run_metric(project, prefix, metric, query, jqfilter):
    """Runs query and filters results, uploading data to GCS."""
    stamp = time.strftime('%Y-%m-%d')
    raw = 'raw-%s.json' % stamp
    filtered = 'daily-%s.json' % stamp
    latest = '%s-latest.json' % metric

    do_query(query, raw, project)
    do_jq(jqfilter, raw, filtered)
    def copy(src, dst):
        """Use gsutil to copy src to test with minimal caching."""
        check([
            'gsutil', '-h', 'Cache-Control:max-age=60', 'cp', src, dst])

    copy(raw, os.path.join(prefix, metric, raw))
    copy(filtered, os.path.join(prefix, metric, filtered))
    copy(filtered, os.path.join(prefix, latest))

def main(configs, project, bucket_path, nonsense=True):
    """Loads metric config files and runs each metric."""
    if not project:
        raise ValueError('project', project)
    if not bucket_path:
        raise ValueError('bucket_path', bucket_path)

    # the 'bq show' command is called as a hack to dodge the config prompts that bq presents
    # the first time it is run. A newline is passed to stdin to skip the prompt for default project
    # when the service account in use has access to multiple projects.
    if nonsense:
        check(['bq', 'show'], stdin='\n')

    errs = []
    root = os.path.join(os.path.dirname(__file__), '../metrics/*.yaml')
    for path in configs or glob.glob(root):
        try:
            with open(path) as config_raw:
                config = yaml.safe_load(config_raw)
            if not config:
                raise ValueError('invalid yaml: %s.' % path)
            metric = config['metric'].strip()
            validate_metric_name(metric)
            run_metric(
                project,
                bucket_path,
                metric,
                config['query'],
                config['jqfilter'],
            )
        except (ValueError, KeyError, IOError, subprocess.CalledProcessError):
            print >>sys.stderr, traceback.format_exc()
            errs += path

    if errs:
        print 'Failed %d configs: %s' % (len(errs), ', '.join(errs))
        sys.exit(1)


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser()
    PARSER.add_argument(
        '--config', action='append', help='yaml file describing a metric.')
    PARSER.add_argument(
        '--project',
        default='k8s-gubernator',
        help='Charge the specified account for bigquery usage')
    PARSER.add_argument(
        '--bucket',
        default='gs://k8s-metrics',
        help='Upload results to the specified gcs bucket.')
    PARSER.add_argument(
        '--human', action='store_true', help='Add this for more fast.')

    ARGS = PARSER.parse_args()
    main(ARGS.config, ARGS.project, ARGS.bucket, not ARGS.human)
