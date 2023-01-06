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

# Need to figure out why this only fails on travis
# pylint: disable=bad-continuation


"""Selects a random sample of kubernetes developers."""

import json
import os
import random
import sys
import functools

import requests

def download_content():
    """Downloads contributor data from github."""
    resp = requests.get('https://api.github.com/repos/kubernetes/kubernetes/stats/contributors')
    resp.raise_for_status()
    data = resp.content
    return data


def load_content(data):
    """Parse the json response."""
    users = [User(b) for b in json.loads(data)]
    return users


@functools.total_ordering
class User:  # pylint: disable=too-few-public-methods
    """Store .user and number of .total and .recent commits."""
    def __init__(self, blob):
        self.user = blob['author']['login']
        weeks = blob['weeks']
        self.recent = sum(k['c'] for k in weeks[-12:])
        self.total = sum(k['c'] for k in weeks)

    def __eq__(self, other):
        return (self.recent, self.total, self.user) == (other.recent, other.total, other.user)

    def __lt__(self, other):
        return (self.recent, self.total, self.user) < (other.recent, other.total, other.user)


def find_users(users, num, top, middle, bottom):
    """Selects num users from top, middle, bottom thirds with specified biases."""
    total = len(users)
    if num >= total:
        return users
    third = int(total/3.0)
    # pylint: disable=invalid-name
    p3 = random.sample(users[:third], int(num * bottom))
    p5 = random.sample(users[third:-third], int(num * middle))
    p7 = random.sample(users[-third:], int(num * top))
    # pylint: enable=invalid-name
    have = []
    have.extend(p3)
    have.extend(p5)
    have.extend(p7)
    if len(have) < num:
        missing = num - len(have)
        remaining = [u for u in users if u not in have]
        extra = random.sample(remaining, missing)
        have.extend(extra)
    return have


def main(path=None, num=35, top=0.6, middle=0.2, bottom=0.2):
    """Select users to survey."""
    if not path:
        data = download_content()
    else:
        with open(os.path.expanduser(path)) as fp:
            data = fp.read()
    users = sorted(load_content(data))
    for user in find_users(users, num, top, middle, bottom):
        print('%s (%d recent commits, %d total)' % (user.user, user.recent, user.total))


if __name__ == '__main__':
    main(*sys.argv[1:])
