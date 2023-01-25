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

# Need to figure out why this only fails on travis
# pylint: disable=bad-continuation

"""update gcloud on Jenkins vms."""

import os
import subprocess
import sys

def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)


def main():
    """update gcloud for Jenkins vms"""
    host = os.environ.get('HOSTNAME')
    if host == 'jenkins-master' or host == 'pull-jenkins-master':
        check('sudo', 'gcloud', 'components', 'update')
        check('sudo', 'gcloud', 'components', 'update', 'beta')
        check('sudo', 'gcloud', 'components', 'update', 'alpha')
    else:
        try:
            check('sudo', 'apt-get', 'update')
        except subprocess.CalledProcessError:
            check('sudo', 'rm', '/var/lib/apt/lists/partial/*')
            check('sudo', 'apt-get', 'update')
        check('sudo', 'apt-get', 'install', '-y', 'google-cloud-sdk')


if __name__ == '__main__':
    main()
