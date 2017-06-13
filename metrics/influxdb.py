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

"""InfluxDB client that pushes time series points from JSON."""

import httplib
import json
import sys
import time
import traceback
import urllib

class Point(object):
    """Represents an InfluxDB time series point and handles serialization."""
    def __init__(self, measurement, tags, fields, time_stamp):
        """Creates a new Point."""
        self.measurement = measurement
        self.tags = tags
        self.fields = fields
        self.time = time_stamp

    @staticmethod
    def from_dict(dct):
        """Deserialization function to be used by json.load()."""
        if any(k not in dct for k in ['measurement', 'tags', 'fields']):
            return dct
        return Point(
            dct['measurement'],
            dct['tags'],
            dct['fields'],
            dct.get('time'),
        )

    def serialize(self):
        """Encodes the Point in InfluxDB Line Protocol syntax.

        For a detailed explanation of Line Protocol's syntax go to:
        https://docs.influxdata.com/influxdb/v1.2/write_protocols/line_protocol_tutorial
        Here is simple example from that tutorial:

        weather,location=us-midwest temperature=82 1465839830100400200
          |    -------------------- --------------  |
          |             |             |             |
          |             |             |             |
        +-----------+--------+-+---------+-+---------+
        |measurement|,tag_set| |field_set| |timestamp|
        +-----------+--------+-+---------+-+---------+
        """
        if not self.fields:
            raise ValueError('Point must have at least one field.\n')

        def value_repr(val):
            """Encodes a field value."""
            if isinstance(val, bool):
                return str(val)
            if isinstance(val, basestring):
                return '\"%s\"' % val.replace('"', '\"')
            return repr(val)

        def measure_repr(measurement):
            """Encodes a measurement name."""
            return measurement.replace(',', r'\,').replace(' ', r'\ ')

        def label_repr(label):
            """Encodes a tag name, tag value, or field name."""
            return label.replace(',', r'\,').replace(' ', r'\ ').replace('=', r'\=')

        # Tag set must be prefixed with, and joined by a comma.
        tags = ''.join([
            ',%s=%s' % (label_repr(key), label_repr(val))
            for key, val in self.tags.iteritems()
        ])
        # Field set is only joined by a comma.
        fields = ','.join([
            '%s=%s' % (label_repr(key), value_repr(val))
            for key, val in self.fields.iteritems()
        ])
        return '%s%s %s%s' % (
            measure_repr(self.measurement),
            tags,
            fields,
            ' %d' % self.time if self.time else ''
        )

class Pusher(object):
    """Client that pushes Point objects to an InfluxDB."""
    def __init__(self, host_port, path, user, password):
        self.host_port = host_port
        self.path = path or ""
        self.user = user
        self.password = password

    @staticmethod
    def from_config(config_path):
        """Creates an Pusher for the InfluxDB described by a json config."""
        with open(config_path) as config_file:
            config = json.load(config_file)

        def check_config(field):
            if not field in config:
                raise ValueError('Pusher config requires field \'%s\'' % field)
        check_config('hostport')
        check_config('user')
        check_config('password')
        return Pusher(
            config['hostport'],
            config.get('path'),
            config['user'],
            config['password']
        )

    def push(self, points, database):
        """Pushes time series data points to an InfluxDB in batches."""
        params = urllib.urlencode(
            {'db': database, 'u': self.user, 'p': self.password, 'precision': 's'}
        )

        stamp = int(time.time())
        for point in points:
            if not point.time:
                point.time = stamp

        while points:
            body = '\n'.join(p.serialize() for p in points[:100])
            points = points[100:]
            for attempt in range(5):
                if attempt:
                    time.sleep(2 ** (attempt - 1))

                try:
                    conn = httplib.HTTPConnection(self.host_port)
                    conn.request('POST', '%s/write?%s' % (self.path, params), body)
                    resp = conn.getresponse()
                except httplib.HTTPException:
                    print >>sys.stderr, (
                        'Exception POSTing influx points to: %s\n%s'
                        % (self.host_port, traceback.format_exc())
                    )
                    continue
                if resp.status >= 500:
                    continue
                if resp.status >= 400:
                    raise Error(
                        'Error writing InfluxDB points (attempt #%d, status code %d): %s'
                        % (attempt, resp.status, resp.read())
                    )
                break
            else:
                raise Error(
                    'Failed to write InfluxDB points with %d attempts. (status code %d): %s'
                    % (attempt, resp.status, resp.read())
                )

class Error(Exception):
    pass
