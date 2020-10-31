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

# pylint: disable=missing-docstring

import unittest

import stream
import make_db_test
import model


class FakeSub:
    def __init__(self, pulls):
        self.pulls = pulls
        self.trace = []

    def pull(self, subscription, return_immediately=False, **_kwargs):
        self.trace.append(['pull', subscription, return_immediately])
        return self.pulls.pop(0)

    def acknowledge(self, subscription, ack_ids):
        self.trace.append(['ack', subscription, ack_ids])

    def modify_ack_deadline(self, subscription, ack_ids, ack_deadline_seconds):
        self.trace.append(['modify-ack', subscription, ack_ids, ack_deadline_seconds])

class FakeClient:
    def __init__(self, trace=None):
        self.trace = [] if trace is None else trace

    def insert_rows(self, table, *args, **kwargs):
        self.trace.append(['insert-rows', args, kwargs])
        return []

class FakeTable:
    def __init__(self, name, schema, trace=None):
        self.name = name
        self.schema = schema

class Attrs:
    def __init__(self, attributes):
        self.attributes = attributes


class FakeSchemaField:
    def __init__(self, **kwargs):
        self.__dict__ = kwargs


class StreamTest(unittest.TestCase):
    def test_main(self):
        # It's easier to run a full integration test with stubbed-out
        # external interfaces and validate the trace than it is to test
        # each individual piece.
        # The components are mostly tested in make_*_test.py.

        db = model.Database(':memory:')
        fakesub = FakeSub([
            [
                ('a', Attrs({'eventType': 'OBJECT_DELETE'})),
            ],
            [
                ('b', Attrs({
                    'eventType': 'OBJECT_FINALIZE',
                    'objectId': 'logs/fake/123/finished.json',
                    'bucketId': 'kubernetes-jenkins'})),
            ],
            [],
            [
                ('c', Attrs({
                    'eventType': 'OBJECT_FINALIZE',
                    'objectId': 'logs/fake/123/finished.json',
                    'bucketId': 'kubernetes-jenkins'})),
            ],
            [],
            [
                ('d', Attrs({
                    'eventType': 'OBJECT_FINALIZE',
                    'objectId': 'logs/fake/124/started.json'})),
            ],
            [],
        ])
        fake_client = FakeClient()
        fake_table = FakeTable('day', stream.load_schema(FakeSchemaField))
        fake_sub_path = 'projects/{project_id}/subscriptions/{sub}'
        tables = {'day': (fake_table, 'incr')}
        stream.main(
            db, fakesub, fake_sub_path, fake_client, tables, make_db_test.MockedClient, [1, 0, 0, 0].pop)

        # uncomment if the trace changes
        import pprint; pprint.pprint(fakesub.trace)
        self.maxDiff = 3000

        now = make_db_test.MockedClient.NOW

        self.assertEqual(
            fakesub.trace,
            [['pull', fake_sub_path, False],
             ['pull', fake_sub_path, True],
             ['pull', fake_sub_path, True],
             ['ack', fake_sub_path, ['a']],
             ['modify-ack', fake_sub_path, ['b'], 180],
             ['ack', fake_sub_path, ['b']],
             ['insert-data',
              ([(5,
                 now - 5,
                 now,
                 True,
                 'SUCCESS',
                 None,
                 'gs://kubernetes-jenkins/logs/fake/123',
                 'fake',
                 123,
                 (),
                 [{'name': 'Foo', 'time': 3.0},
                  {'failed': True,
                   'failure_text': 'stacktrace',
                   'name': 'Bad',
                   'time': 4.0}],
                 2,
                 1,
                 None,
                 None,
                 None)],
               [1]),
              {'skip_invalid_rows': True}],
             ['pull', fake_sub_path, False],
             ['pull', fake_sub_path, True],
             ['modify-ack', fake_sub_path, ['c'], 180],
             ['ack', fake_sub_path, ['c']],
             ['pull', fake_sub_path, False],
             ['pull', fake_sub_path, True],
             ['ack', fake_sub_path, ['d']]])


if __name__ == '__main__':
    unittest.main()
