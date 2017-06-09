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
import json
import os
import re
import subprocess
import sys
import time
import traceback

import yaml

import influxdb

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
    """Raise ValueError if name is non-trivial."""
    # Regex '$' symbol matches an optional terminating new line
    # so we have to check that the name
    # doesn't have one if the regex matches.
    if not re.match(r'^[\w-]+$', name) or name[-1] == '\n':
        raise ValueError(name)

def do_query(query, out_filename, project):
    """Executes a bigquery query, outputting the results to a file."""
    with open(out_filename, 'w') as out_file:
        cmd = [
            'bq', 'query', '--format=prettyjson', '--project_id=%s' % project,
            '-n100000',  # Results may have more than 100 rows
        ]
        check(cmd, stdin=query, stdout=out_file)

def do_jq(jqfilter, data_filename, out_filename, jq_bin='jq'):
    """Executes jq on a data file and outputs the results to a file."""
    with open(out_filename, 'w') as out_file:
        check([jq_bin, jqfilter, data_filename], stdout=out_file)

def run_metric(project, prefix, metric, query, jqfilter, jq_point=None, influx=None):
    """Runs query and filters results, uploading data to GCS."""
    stamp = time.strftime('%Y-%m-%d')
    raw = 'raw-%s.json' % stamp
    filtered = 'daily-%s.json' % stamp
    latest = '%s-latest.json' % metric
    points = '%s-data-points.json' % metric

    do_query(query, raw, project)
    do_jq(jqfilter, raw, filtered)
    if jq_point:
        if influx:
            do_jq(jq_point, raw, points)
            with open(points) as points_file:
                points = json.load(
                    points_file,
                    object_hook=influxdb.Point.from_dict,
                )
            influx.push(points, "metrics")
        else:
            print >>sys.stderr, 'Skipping upload of %s, no influxdb config.\n' % metric

    def copy(src, dst):
        """Use gsutil to copy src to test with minimal caching."""
        check([
            'gsutil', '-h', 'Cache-Control:max-age=60', 'cp', src, dst])

    copy(raw, os.path.join(prefix, metric, raw))
    copy(filtered, os.path.join(prefix, metric, filtered))
    copy(filtered, os.path.join(prefix, latest))

def all_configs(search='**.yaml'):
    """Returns config files in the metrics dir."""
    return glob.glob(os.path.join(
        os.path.dirname(__file__), 'configs', search))

def main(configs, project, bucket_path):
    """Loads metric config files and runs each metric."""
    if not project:
        raise ValueError('project', project)
    if not bucket_path:
        raise ValueError('bucket_path', bucket_path)

    if 'VELODROME_INFLUXDB_CONFIG' in os.environ:
        influx = influxdb.Pusher.from_config(os.environ['VELODROME_INFLUXDB_CONFIG'])
    else:
        influx = None

    # the 'bq show' command is called as a hack to dodge the config prompts that bq presents
    # the first time it is run. A newline is passed to stdin to skip the prompt for default project
    # when the service account in use has access to multiple projects.
    check(['bq', 'show'], stdin='\n')

    errs = []
    for path in configs or all_configs():
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
                config.get('jqmeasurements'),
                influx,
            )
        except (
                ValueError,
                KeyError,
                IOError,
                subprocess.CalledProcessError,
                influxdb.Error,
            ):
            print >>sys.stderr, traceback.format_exc()
            errs.append(path)

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

    ARGS = PARSER.parse_args()
    main(ARGS.config, ARGS.project, ARGS.bucket)
