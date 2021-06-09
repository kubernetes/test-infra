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

"""Receive push events for new builds and upload rows to BigQuery."""



import argparse
import json
import os
import pprint
import socket
import sys
import traceback
import time

import multiprocessing.pool
import ruamel.yaml as yaml

try:
    from google.api_core import exceptions as api_exceptions
    from google.cloud import bigquery
    from google.cloud import pubsub_v1
    import google.cloud.exceptions
except ImportError:
    print('WARNING: unable to load google cloud (test environment?)')
    traceback.print_exc()

import model
import make_db
import make_json


MAX_ROW_UPLOAD = 10 # See https://github.com/googleapis/google-cloud-go/issues/2855


def should_exclude(object_id, bucket_id, buckets):
    # Objects of form a/b/c/<jobname>/<hash>/<objectFile>'
    if bucket_id not in buckets:
        return False
    return any(f'/{job}/' in object_id for job in buckets[bucket_id].get('exclude_jobs', []))


def process_changes(results, buckets):
    """Split GCS change events into trivial ack_ids and builds to further process."""
    ack_ids = []  # pubsub rec_message ids to acknowledge
    todo = []  # (id, job, build) of builds to grab
    # process results, find finished builds to process
    for rec_message in results:
        object_id = rec_message.message.attributes['objectId']
        bucket_id = rec_message.message.attributes['bucketId']
        exclude = should_exclude(object_id, bucket_id, buckets)
        if not object_id.endswith('/finished.json') or exclude:
            ack_ids.append(rec_message.ack_id)
            continue
        job, build = object_id[:-len('/finished.json')].rsplit('/', 1)
        job = 'gs://%s/%s' % (bucket_id, job)
        todo.append((rec_message.ack_id, job, build))
    return ack_ids, todo


def get_started_finished(gcs_client, db, todo):
    """Download started/finished.json from build dirs in todo."""
    ack_ids = []
    build_dirs = []
    pool = multiprocessing.pool.ThreadPool(16)
    try:
        for ack_id, (build_dir, started, finished) in pool.imap_unordered(
                lambda ack_id_job_build: (ack_id_job_build[0], gcs_client.get_started_finished(
                    ack_id_job_build[1], ack_id_job_build[2])),
                todo):
            if finished:
                if not db.insert_build(build_dir, started, finished):
                    print('build dir already present in db: ', build_dir)
                start = time.localtime(started.get('timestamp', 0) if started else 0)
                print((build_dir, bool(started), bool(finished),
                       time.strftime('%F %T %Z', start),
                       finished and finished.get('result')))
                build_dirs.append(build_dir)
                ack_ids.append(ack_id)
            else:
                print('finished.json missing?', build_dir, started, finished)
    finally:
        pool.close()
    db.commit()
    return ack_ids, build_dirs


def retry(func, *args, **kwargs):
    """Run a function with arguments, retrying on server errors. """
    # pylint: disable=no-member
    for attempt in range(20):
        try:
            return func(*args, **kwargs)
        except (socket.error, google.cloud.exceptions.ServerError):
            # retry with exponential backoff
            traceback.print_exc()
            time.sleep(1.4 ** attempt)
        except api_exceptions.BadRequest as err:
            args_size = sys.getsizeof(args)
            kwargs_str = ','.join('{}={}'.format(k, v) for k, v in kwargs.items())
            print(f"Error running {func.__name__} \
                   ([bytes in args]{args_size} with {kwargs_str}) : {err}")
            return None # Skip
    return func(*args, **kwargs)  # one last attempt


def insert_data(bq_client, table, rows_iter):
    """Upload rows from rows_iter into bigquery table table.

    rows_iter should return a series of (row_id, row dictionary) tuples.
    The row dictionary must match the table's schema.

    Args:
        bq_client: Client connection to BigQuery
        table: bigquery.Table object that points to a specific table
        rows_iter: row_id, dict representing a make_json.Build
    Returns the row_ids that were inserted.
    """
    def divide_chunks(l, bin_size=MAX_ROW_UPLOAD):
        # break up rows to not hit data limits
        for i in range(0, len(l), bin_size):
            yield l[i:i + bin_size]

    emitted, rows = [], []

    for row_id, build in rows_iter:
        emitted.append(row_id)
        rows.append(build)

    if not rows:  # nothing to do
        return []

    for chunk in divide_chunks(rows):
        # Insert rows with row_ids into table, retrying as necessary.
        errors = retry(bq_client.insert_rows, table, chunk, skip_invalid_rows=True)
        if not errors:
            print(f'Loaded {len(chunk)} builds into {table.full_table_id}')
        else:
            print(f'Errors on Chunk: {chunk}')
            pprint.pprint(errors)
            pprint.pprint(table.schema)

    return emitted


def main(
        db,
        subscriber,
        subscription_path,
        bq_client,
        tables,
        buckets,
        client_class=make_db.GCSClient,
        stop=None,
    ):
    # pylint: disable=too-many-locals
    gcs_client = client_class('', {})
    if stop is None:
        stop = lambda: False

    results = [0] * 1000  # don't sleep on first loop
    while not stop():
        print()
        if len(results) < 10 and client_class is make_db.GCSClient:
            time.sleep(5)  # slow down!

        print('====', time.strftime("%F %T %Z"), '=' * 40)

        results = retry(subscriber.pull, subscription=subscription_path, max_messages=1000)
        results = list(results.received_messages)
        start = time.time()
        while time.time() < start + 7:
            results_more = list(subscriber.pull(
                subscription=subscription_path,
                max_messages=1000,
                return_immediately=True).received_messages)
            if not results_more:
                break
            results.extend(results_more)

        print('PULLED', len(results))

        ack_ids, todo = process_changes(results, buckets)

        if ack_ids:
            print('ACK irrelevant', len(ack_ids))
            for n in range(0, len(ack_ids), 1000):
                retry(
                    subscriber.acknowledge,
                    subscription=subscription_path,
                    ack_ids=ack_ids[n: n + 1000])

        if todo:
            print('EXTEND-ACK ', len(todo))
            # give 3 minutes to grab build details
            retry(
                subscriber.modify_ack_deadline,
                subscription=subscription_path,
                ack_ids=[i for i, _j, _b in todo],
                ack_deadline_seconds=60*3)

        ack_ids, build_dirs = get_started_finished(gcs_client, db, todo)

        # notify pubsub queue that we've handled the finished.json messages
        if ack_ids:
            print('ACK "finished.json"', len(ack_ids))
            retry(subscriber.acknowledge, subscription=subscription_path, ack_ids=ack_ids)

        # grab junit files for new builds
        make_db.download_junit(db, 16, client_class)

        # stream new rows to tables
        if build_dirs and tables:
            for table, incremental_table in tables.values():
                builds = db.get_builds_from_paths(build_dirs, incremental_table)
                emitted = insert_data(bq_client, table, make_json.make_rows(db, builds))
                db.insert_emitted(emitted, incremental_table)


def load_sub(poll):
    """Return the PubSub subscription specified by the /-separated input.

    Args:
        poll: Follow GCS changes from project/topic/subscription
              Ex: kubernetes-jenkins/gcs-changes/kettle

    Return:
        Subscribed client
    """
    subscriber = pubsub_v1.SubscriberClient()
    project_id, _, sub = poll.split('/')
    subscription_path = f'projects/{project_id}/subscriptions/{sub}'
    return subscriber, subscription_path


def load_schema(schemafield):
    """Construct the expected BigQuery schema from files on disk.

    Only used for new tables."""
    basedir = os.path.dirname(__file__)
    with open(os.path.join(basedir, 'schema.json')) as json_file:
        schema_json = json.load(json_file)
    def make_field(spec):
        spec['field_type'] = spec.pop('type')
        if 'fields' in spec:
            spec['fields'] = [make_field(f) for f in spec['fields']]
        return schemafield(**spec)
    return [make_field(s) for s in schema_json]


def load_tables(dataset, tablespecs):
    """Construct a dictionary of BigQuery tables given the input tablespec.

    Args:
        dataset: bigquery.Dataset
        tablespecs: list of strings of "NAME:DAYS", e.g. ["day:1"]
    Returns:
        client, {name: (bigquery.Table, incremental table name)}
    """
    project, dataset_name = dataset.split(':')
    bq_client = bigquery.Client(project)

    tables = {}
    for spec in tablespecs:
        table_name, days = spec.split(':')
        table_ref = f'{project}.{dataset_name}.{table_name}'
        try:
            table = bq_client.get_table(table_ref) # pylint: disable=no-member
        except google.cloud.exceptions.NotFound:
            table = bq_client.create_table(table_ref) # pylint: disable=no-member
            table.schema = load_schema(bigquery.schema.SchemaField)
        tables[table_name] = (table, make_json.get_table(float(days)))
    return bq_client, tables


class StopWhen:
    """A simple object that returns True once when the given hour begins."""
    def __init__(self, target, clock=lambda: time.localtime().tm_hour):
        self.clock = clock
        self.last = self.clock()
        self.target = target

    def __call__(self):
        if os.path.exists('stop'):
            return True
        now = self.clock()
        last = self.last
        self.last = now
        return now != last and now == self.target


def _make_bucket_map(path):
    bucket_map = yaml.safe_load(open(path))
    bucket_to_attrs = dict()
    for k, v in bucket_map.items():
        bucket = k.rsplit('/')[2] # of form gs://<bucket>/...
        bucket_to_attrs[bucket] = v
    return bucket_to_attrs

def get_options(argv):
    """Process command line arguments."""
    parser = argparse.ArgumentParser()
    parser.add_argument(
        '--poll',
        required=True,
        help='Follow GCS changes from project/topic/subscription',
    )
    parser.add_argument(
        '--dataset',
        help='BigQuery dataset (e.g. k8s-gubernator:build)'
    )
    parser.add_argument(
        '--tables',
        nargs='+',
        default=[],
        help='Upload rows to table:days [e.g. --tables day:1 week:7 all:0]',
    )
    parser.add_argument(
        '--stop_at',
        type=int,
        help='Terminate when this hour (0-23) rolls around (in local time).'
    )
    parser.add_argument(
        '--buckets',
        type=str,
        default='buckets.yaml',
        help='Path to bucket configuration.'
    )
    return parser.parse_args(argv)


if __name__ == '__main__':
    OPTIONS = get_options(sys.argv[1:])
    main(model.Database(),
         *load_sub(OPTIONS.poll),
         *load_tables(OPTIONS.dataset, OPTIONS.tables),
         _make_bucket_map(OPTIONS.buckets),
         stop=StopWhen(OPTIONS.stop_at))
