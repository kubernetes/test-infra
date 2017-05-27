#!/usr/bin/env python2

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

import cStringIO as StringIO
import json
import unittest

import stream
import make_db_test
import model
import time

import pprint


class FakeSub(object):
    def __init__(self, pulls):
        self.pulls = pulls
        self.trace = []

    def pull(self, **kwargs):
        self.trace.append(['pull'])
        return self.pulls.pop(0)

    def acknowledge(self, acks):
        self.trace.append(['ack', acks])

    def modify_ack_deadline(self, acks, time):
        self.trace.append(['modify-ack', acks, time])


class FakeTable(object):
    def __init__(self, name, schema, trace=None):
        self.name = name
        self.schema = schema
        self.trace = [] if trace is None else trace

    def insert_data(self, *args, **kwargs):
        self.trace.append(['insert-data', args, kwargs])
        return []


class Attrs(object):
    def __init__(self, attributes):
        self.attributes = attributes


class FakeSchemaField(object):
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
                ('a', Attrs({'event_type': 'OBJECT_DELETE'})),
                ('b', Attrs({
                    'event_type': 'OBJECT_FINALIZE',
                    'object_id': 'logs/fake/123/finished.json',
                    'bucket_id': 'kubernetes-jenkins'})),
            ],
            [
                ('c', Attrs({
                    'event_type': 'OBJECT_FINALIZE',
                    'object_id': 'logs/fake/123/finished.json',
                    'bucket_id': 'kubernetes-jenkins'})),
            ],
            [
                ('d', Attrs({
                    'event_type': 'OBJECT_FINALIZE',
                    'object_id': 'logs/fake/124/started.json'})),
            ]
        ])
        faketable = FakeTable('day', stream.load_schema(FakeSchemaField), fakesub.trace)
        tables = {'day': (faketable, 'incr')}
        stream.main(
            db, fakesub, tables, make_db_test.MockedClient, [1, 0, 0, 0].pop)

        # pprint.pprint(fakesub.trace)

        now = make_db_test.MockedClient.NOW

        self.maxDiff = 3000

        self.assertEqual(fakesub.trace,
            [['pull'],
             ['ack', ['a']],
             ['modify-ack', ['b'], 180],
             ['ack', ['b']],
             ['insert-data',
              ([[5,
                 now - 5,
                 now,
                 u'SUCCESS',
                 None,
                 u'gs://kubernetes-jenkins/logs/fake/123',
                 u'fake',
                 123,
                 [],
                 [{'name': 'Foo', 'time': 3.0},
                  {'failed': True,
                   'failure_text': 'stacktrace',
                   'name': 'Bad',
                   'time': 4.0}],
                 2,
                 1,
                 None]],
               [1]),
              {'skip_invalid_rows': True}],
             ['pull'],
             ['modify-ack', ['c'], 180],
             ['ack', ['c']],
             ['insert-data', ([], []), {'skip_invalid_rows': True}],
             ['pull'],
             ['ack', ['d']]]
        )


if __name__ == '__main__':
    unittest.main()
