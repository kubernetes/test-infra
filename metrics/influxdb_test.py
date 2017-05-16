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

"""Test influxdb.py."""

import BaseHTTPServer
import threading
import re
import unittest

import influxdb

class TestInfluxPoint(unittest.TestCase):
    def test_from_dict(self):
        def check(sample, measurement, tags, fields, time):
            point = influxdb.Point.from_dict(sample)
            self.assertEqual(point.measurement, measurement)
            self.assertEqual(point.tags, tags)
            self.assertEqual(point.fields, fields)
            self.assertEqual(point.time, time)

        check(
            {
                'measurement': 'metric',
                'tags': {'style': 'stylish'},
                'fields': {'baseball': 'diamond', 'basketball': False},
                'time': 42
            },
            'metric',
            {'style': 'stylish'},
            {'baseball': 'diamond', 'basketball': False},
            42,
        )
        check(
            {
                'measurement': 'metric',
                'tags': {},
                'fields': {'num': 2.7},
            },
            'metric',
            {},
            {'num': 2.7},
            None,
        )
        # Check that objects that don't meet the InfluxPoint spec are unchanged.
        sample = {
            'measurement': 'metric',
            'tags': {'tag': 'value'},
            'notfields': 'something',
        }
        self.assertEqual(influxdb.Point.from_dict(sample), sample)

    def test_serialize(self):
        def check(measurement, tags, fields, time, expected):
            point = influxdb.Point(measurement, tags, fields, time)
            self.assertEqual(point.serialize(), expected)

        check(
            'metric',
            {'type': 'good'},
            {'big?': True, 'size': 20},
            42,
            'metric,type=good big?=True,size=20 42',
        )
        check(
            'measure with spaces',
            {'tag,with,comma': 'tagval=with=equals'},
            {',,': 20.2, 'string': 'yarn'},
            None,
            r'measure\ with\ spaces,tag\,with\,comma=tagval\=with\=equals \,\,=20.2,string="yarn"',
        )
        check(
            'measure with spaces',
            {'tag,with,comma': 'tagval=with=equals'},
            {',,': 20.2, 'string': 'yarn'},
            None,
            r'measure\ with\ spaces,tag\,with\,comma=tagval\=with\=equals \,\,=20.2,string="yarn"',
        )

class RequestHandler(BaseHTTPServer.BaseHTTPRequestHandler):
    def do_POST(self): # pylint: disable=invalid-name
        if not self.path.startswith('/write'):
            raise ValueError(
                'path should start with \'/write\', but is \'%s\'' % self.path
            )
        body = self.rfile.read(int(self.headers.getheader('content-length')))
        new_ids = [int(match) for match in re.findall(r'id=(\d+)', body)]
        self.server.received = self.server.received.union(new_ids)

        self.send_response(201)

class TestInfluxPusher(unittest.TestCase):
    def setUp(self):
        self.port = 8000
        self.written = 0
        self.test_server = BaseHTTPServer.HTTPServer(
            ('', self.port),
            RequestHandler,
        )
        self.test_server.received = set()
        thread = threading.Thread(target=self.test_server.serve_forever)
        thread.start()

    def tearDown(self):
        self.test_server.shutdown()
        for num in xrange(self.written):
            self.assertIn(num, self.test_server.received)

    def test_push(self):
        points = [influxdb.Point('metric', {}, {'id': num}, None)
                  for num in xrange(110)]
        pusher = influxdb.Pusher(
            'localhost:%d' % self.port,
            None,
            'username',
            'pass123',
        )
        pusher.push(points, 'mydb')
        self.written = 110

if __name__ == '__main__':
    unittest.main()
