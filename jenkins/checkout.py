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


import argparse
import subprocess


def Checkout(git, org, repo, branch, pull):
    if bool(branch) == bool(pull):
        raise ValueError('Must specify one of --branch or --pull')
    if pull:
        ref = '+refs/pull/%d/merge' % pull
    else:
        ref = branch

    subprocess.check_call([git, 'init', repo])
    subprocess.check_call([
        git,
        '-C', repo,
        'fetch', 'https://github.com/%s/%s' % (org, repo), ref,
    ])
    subprocess.check_call([git, '-C', repo, 'checkout', 'FETCH_HEAD'])

if __name__ == '__main__':
  parser = argparse.ArgumentParser('Checks out a github PR/branch to ./<repo>/')
  parser.add_argument('--git', default='git', help='Path to git')
  parser.add_argument('--org', default='kubernetes', help='Checkout from the following org')
  parser.add_argument('--pull', type=int, help='PR number')
  parser.add_argument('--branch', help='Checkout the following branch')
  parser.add_argument('--repo', required=True, help='The kubernetes repository to fetch from')
  args = parser.parse_args()
  Checkout(args.git, args.org, args.repo, args.branch, args.pull)
