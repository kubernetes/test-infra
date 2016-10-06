#!/usr/bin/env python

# Copyright 2016 The Kubernetes Authors.
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

"""Remove Jenkins agent SSH keys from a project's metadata."""

from __future__ import print_function

import argparse
import json
import requests
import subprocess
import sys
import time


AGENT_PR_SUBSTR = 'agent-pr'
NUM_RETRIES = 6
SLEEP_DURATION = 10


def try_update(project, token):
    """Try to remove agent keys. Returns True on success."""
    get_url = 'https://www.googleapis.com/compute/v1/projects/%s' % project
    headers = {
        'Authorization': 'Bearer %s' % token,
        'Content-Type': 'application/json'
    }
    r = requests.get(get_url, headers=headers)
    # Retry if the get failed.
    if r.status_code != 200:
        print('Got status code %d on GET.' % r.status_code)
        print(r.text)
        return False
    json_data = r.json()
    fingerprint = json_data['commonInstanceMetadata']['fingerprint']
    old_keys = []
    for entry in json_data['commonInstanceMetadata']['items']:
        if entry['key'] == 'sshKeys':
            old_keys = entry['value'].splitlines()
    # No ssh keys in the response. Nothing to do.
    if not old_keys:
        return True
    new_keys = [line for line in old_keys if AGENT_PR_SUBSTR not in line]
    if len(new_keys) == len(old_keys):
        # No keys removed. Nothing to do.
        return True
    else:
        print('Attempting to remove %d keys.' % (len(old_keys) - len(new_keys)))
    resp_data = {
        "kind": "compute#metadata",
        "fingerprint": fingerprint,
        "items": [
            {
                "key": "sshKeys",
                "value": '\n'.join(new_keys)
            }
        ]
    }
    post_url = '%s/setCommonInstanceMetadata' % get_url
    r = requests.post(post_url, headers=headers, data=json.dumps(resp_data))
    # Retry if the fingerprint is wrong.
    if r.status_code == 412:
        return False
    # Don't retry on 200.
    if r.status_code == 200:
        return True
    # Retry on other status codes.
    print('Got status code %d on POST.' % r.status_code)
    print(r.text)
    return False


def main(project):
    token = subprocess.check_output([
        'gcloud',
        'auth',
        'application-default',
        'print-access-token']).strip()
    for retry in range(NUM_RETRIES):
        if try_update(project, token):
            return 0
        else:
            time.sleep(SLEEP_DURATION)
    return 1


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Delete Jenkins SSH keys.')
    parser.add_argument('--project', help='Project to clean', required=True)
    args = parser.parse_args()

    sys.exit(main(args.project))
