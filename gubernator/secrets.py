#!/usr/bin/env python

# Copyright 2018 The Kubernetes Authors.
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

import os

from google.appengine.ext import ndb

from google.appengine.api import modules
from google.appengine.api import app_identity

def get_hostname():
    hostname = app_identity.get_default_version_hostname()
    if 'testbed' not in os.environ.get('SERVER_SOFTWARE', 'testbed'):
        current_version = modules.modules.get_current_version_name()
        if current_version and modules.modules.get_default_version() != current_version:
            hostname = '%s-dot-%s' % (current_version, hostname)
    return hostname

class Secret(ndb.Model):
    value = ndb.JsonProperty()

    @staticmethod
    def make_key(key, per_host):
        if per_host:
            return ndb.Key(Secret, '%s\t%s' % (get_hostname(), key))
        return ndb.Key(Secret, key)

    @staticmethod
    def make(key, value, per_host):
        return Secret(key=Secret.make_key(key, per_host), value=value)


def get(key, per_host=True):
    data = Secret.make_key(key, per_host).get()
    if not data:
        raise KeyError
    return data.value


def put(key, value, per_host=True):
    Secret.make(key, value, per_host).put()
