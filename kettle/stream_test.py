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

# pylint: disable=missing-docstring

import unittest

import stream
import make_db_test
import model


class FakeSub(object):
    def __init__(self, pulls):
        self.pulls = pulls
        self.trace = []

    def pull(self, return_immediately=False, **_kwargs):
        self.trace.append(['pull', return_immediately])
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
        faketable = FakeTable('day', stream.load_schema(FakeSchemaField), fakesub.trace)
        tables = {'day': (faketable, 'incr')}
        stream.main(
            db, fakesub, tables, make_db_test.MockedClient, [1, 0, 0, 0].pop)

        # uncomment if the trace changes
        # import pprint; pprint.pprint(fakesub.trace)
        # self.maxDiff = 3000

        now = make_db_test.MockedClient.NOW

        self.assertEqual(
            fakesub.trace,
            [['pull', False], ['pull', True], ['pull', True],
             ['ack', ['a']],
             ['modify-ack', ['b'], 180],
             ['ack', ['b']],
             ['insert-data',
              ([[5,
                 now - 5,
                 now,
                 True,
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
             ['pull', False], ['pull', True],
             ['modify-ack', ['c'], 180],
             ['ack', ['c']],
             ['pull', False], ['pull', True],
             ['ack', ['d']]])


if __name__ == '__main__':
    unittest.main()
