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

"""Cancel old, duplicate tests of the same pr.

This can happen with repeated '@k8s-bot test this' commands, or
even something as simple as pushing new code while builds are already queued.
"""

import sys

import requests


def prune(host):
    queue = requests.get('http://%s/queue/api/json' % host).json()

    dupes = {}
    for item in queue['items']:
        params = {}
        for action in item['actions']:
            if 'parameters' in action:
                params = {d['name']: d['value'] for d in action['parameters']}
        if 'PULL_NUMBER' in params:
            name = item['task']['name']
            queueId = item['id']
            since = item['inQueueSince']
            dupes.setdefault((name, params['PULL_NUMBER']), []).append((since, queueId, params))

    for k, v in dupes.iteritems():
        if len(v) < 2:
            continue
        v.sort()
        print '===', k
        for p in v[:-1]:
            print p
            resp = requests.post('http://%s/queue/cancelItem?id=%d' % (host, p[1]))
        print 'GOOD:', v[-1]


if __name__ == '__main__':
    host = sys.argv[1] if len(sys.argv) > 1 else 'localhost:8080'
    prune(host)
