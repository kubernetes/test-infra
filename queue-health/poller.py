#!/usr/bin/env python

# Copyright 2016 The Kubernetes Authors All rights reserved.
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

from __future__ import print_function

import datetime
import requests
import time

def get_submit_queue_json(path):
    return requests.get('http://submit-queue.k8s.io/{}'.format(path)).json()

def is_blocked():
    ci = get_submit_queue_json('health')
    return ci['MergePossibleNow'] != True

def get_stats():
    stats = get_submit_queue_json('sq-stats')
    return stats['Initialized'] == True, stats['MergesSinceRestart']

def poll():
    prs = get_submit_queue_json('prs')
    e2e = get_submit_queue_json('github-e2e-queue')
    online, mergeCount = get_stats()
    return online, len(prs['PRStatus']), len(e2e['E2EQueue']), len(e2e['E2ERunning']), is_blocked(), mergeCount

with open('results.txt', 'a') as f:
    while True:
        try:
            now = datetime.datetime.now()
            try:
                online, prs, queue, running, blocked, mergeCount = poll()
            except KeyboardInterrupt:
                raise
            except Exception:
                online, prs, queue, running, blocked, mergeCount = False, 0, 0, 0, False, 0
            data = '{} {} {} {} {} {} {}'.format(now, online, prs, queue, running, blocked, mergeCount)
            print(data)
            print(data, file=f)
            f.flush()
            time.sleep(60)
        except KeyboardInterrupt:
            break
