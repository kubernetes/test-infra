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

import cStringIO
import datetime
import subprocess
import sys
import time
import traceback

import requests
from pprint import pprint

def get_submit_queue_json(path):
    for n in range(3):
        uri = 'http://submit-queue.k8s.io/%s' % path
        print >>sys.stderr, 'GET %s' % uri
        resp = requests.get(uri)
        if resp.ok:
            break
        time.sleep(2**n)
    resp.raise_for_status()
    return resp.json()


def is_blocked():
    ci = get_submit_queue_json('health')
    return ci['MergePossibleNow'] != True


def get_stats():
    stats = get_submit_queue_json('sq-stats')
    return stats['Initialized'] == True, stats['MergesSinceRestart']


def poll():
    prs = get_submit_queue_json('prs')
    e2e = get_submit_queue_json('github-e2e-queue')
    online, merge_count = get_stats()
    return (
        online,  # Is mergebot initialized?
        len(prs['PRStatus']),  # number of open PRs
        len(e2e['E2EQueue']),  # number of items in the e2e queue
        len(e2e['E2ERunning']),  # Worthless: number of keys in this dict.
        is_blocked(),  # Whether we can merge
        merge_count,  # Number of merges the bot has done
    )


def load_stats(uri):
    while True:
        try:
            return subprocess.check_output(['gsutil', '-q', 'cat', uri])
        except subprocess.CalledProcessError:
            traceback.print_exc()
            time.sleep(5)


def save_stats(uri, buf):
    proc = subprocess.Popen(
        # TODO(fejta): add -Z if this gets resolved:
        # https://github.com/GoogleCloudPlatform/gsutil/issues/364
        ['gsutil', '-q', '-h', 'Content-Type:text/plain',
         'cp', '-a', 'public-read', '-', uri],
        stdin=subprocess.PIPE)
    proc.communicate(buf.getvalue())
    code = proc.wait()
    if code:
      print >>sys.stderr, 'Failed to copy stats to %s: %d' % (uri, code)


def poll_forever(uri, service_account=None):
    if service_account:
      print >>sys.stderr, 'Activating service account using: %s' % service_account
      subprocess.check_call(
          ['gcloud', 'auth', 'activate-service-account', '--key-file=%s' % service_account])
    print >>sys.stderr, 'Loading historical stats from %s...' % uri
    buf = cStringIO.StringIO()
    buf.write(load_stats(uri))
    secs = 60

    while True:
        try:
            print >>sys.stderr, 'Waiting %ds...' % secs
            time.sleep(secs)
            now = datetime.datetime.now()
            print >>sys.stderr, 'Polling current status...'
            online, prs, queue, running, blocked, merge_count = False, 0, 0, 0, False, 0
            try:
                online, prs, queue, running, blocked, merge_count = poll()
            except KeyboardInterrupt:
                raise
            except (KeyError, IOError):
                traceback.print_exc()
                continue

            data = '{} {} {} {} {} {} {}\n'.format(now, online, prs, queue, running, blocked, merge_count)
            print >>sys.stderr, 'Appending to history: %s' % data
            buf.write(data)

            print >>sys.stderr, 'Saving historical stats to %s...' % uri
            save_stats(uri, buf)
        except KeyboardInterrupt:
            break


if __name__ == '__main__':
    # log all arguments.
    pprint(sys.argv)

    poll_forever(*sys.argv[1:])
