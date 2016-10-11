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


import ConfigParser
import sys

from jenkinsapi import jenkins  # sudo pip install jenkinsapi

EXCLUSIVE = True
SHARED = False

# TODO: Add 'scalability' label to heavy to not abuse 'build' label.
# TODO(fejta): add light/heavy/pr to tags and replace nodes
INFO = {
    'heavy': ('build unittest', 1, EXCLUSIVE),
    'light': ('node e2e', 10, EXCLUSIVE),
    'pr': ('pull', 1, SHARED),
}


def info(host, kind):
    labels, executors, exclusive = INFO[kind]
    return {
        'credential_description': 'Jenkins GCE ssh key',
        'exclusive': exclusive,
        'host': host,
        'java_path': '',
        'jvm_options': '',
        'labels': labels,
        'max_num_retries': 0,
        'node_description': '',
        'num_executors': executors,
        'port': 22,
        'prefix_start_slave_cmd': '',
        'remote_fs': '/var/lib/jenkins',
        'retry_wait_time': 0,
        'suffix_start_slave_cmd': '',
    }


def create(api, host, config):
    delete(api, host)
    print 'Creating %s...' % host,
    print api.nodes.create_node(host, config)


def delete(api, host):
    if host in api.nodes:
        print 'Deleting %s...' % host,
        print api.delete_node(host)


def creds(path, section):
    """An ini file with a section per master.

    Should look something like this:
      [master-a]
      user=foo
      key=7deadbeef1234098

      [master-b]
      user=bar
      key=7deadbeef9999999
    """
    config = ConfigParser.SafeConfigParser()
    config.read(ini)
    return config.get(section, 'user'), config.get(section, 'key')


if __name__ == '__main__':
    cmd, host, kind, ini, agent = sys.argv[1:]
    user, key = creds(ini, agent)
    J = jenkins.Jenkins('http://localhost:8080', user, key)

    if sys.argv[1] == 'delete':
        delete(J, host)
    else:
        create(J, host, info(host, kind))
