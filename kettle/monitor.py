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

"""
A dead-simple Influxdb data pusher to report BigQuery database statistics.
"""


import argparse
import json
import sys
import time

try:
    from google.cloud import bigquery
    import google.cloud.exceptions
except ImportError:
    print('WARNING: unable to load google cloud (test environment?)')
    import traceback
    traceback.print_exc()


def collect(tables, stale_hours):
    stale = False
    for table_spec in tables:
        print(f'Checking {table_spec}...')
        project, dataset_name = table_spec.split(':')
        dataset, name = dataset_name.split('.')

        table = bigquery.Client(project).dataset(dataset).table(name)
        try:
            table.reload()
        except google.cloud.exceptions.NotFound:  # pylint: disable=no-member
            continue

        # converting datetimes back into epoch-milliseconds is tiresome
        # pylint: disable=protected-access
        fields = {
            'table_spec': table_spec,
            'size_bytes': table.num_bytes,
            'modified_time': int(table._properties.get('lastModifiedTime')),
            'row_count': table.num_rows
        }
        hours_old = (time.time() - fields['modified_time'] / 1000) / (3600.0)
        fields['hours_old'] = hours_old

        if stale_hours and hours_old > stale_hours:
            print('ERROR: table %s is %.1f hours old. Max allowed: %s hours.' % (
                table.table_id, hours_old, stale_hours))
            stale = True
        print(json.dumps(fields))

    print(f'Finished checking tables')

    return int(stale)


def main(args):
    parser = argparse.ArgumentParser()
    parser.add_argument('--table', nargs='+', required=True,
                        help='List of datasets to return information about.')
    parser.add_argument('--stale', type=int,
                        help='Number of hours to consider stale.')
    opts = parser.parse_args(args)
    return collect(opts.table, opts.stale)


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
