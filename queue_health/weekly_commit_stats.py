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

"""Lists weekly commits for a github org."""

import collections
import datetime
import json
import sys


import requests  # pip install requests


SESSION = requests.Session()

if len(sys.argv) != 3:
    print >>sys.stderr, 'Usage: %s <path/to/token> <org>' % sys.argv[0]
    print >>sys.stderr, '  Info at https://github.com/settings/tokens'
    sys.exit(1)

TOKEN = open(sys.argv[1]).read().strip()


def get(path):
    """Get the specified github api using TOKEN."""
    return SESSION.get(
        'https://api.github.com/%s' % path,
        headers={'Authorization': 'token %s' % TOKEN})


def github_repos(org):
    """List repos for the org."""
    print >>sys.stderr, 'Repos', org
    resp = get('orgs/%s/repos' % org)
    resp.raise_for_status()
    return json.loads(resp.content)


def github_commits(owner, repo):
    """List weekly commits for the repo."""
    print >>sys.stderr, 'Commits', owner, repo
    resp = get('repos/%s/%s/stats/commit_activity' % (owner, repo))
    resp.raise_for_status()
    return json.loads(resp.content)

def org_commits(org):
    """Combine weekly commits for all repos in the org."""
    repos = [(r['owner']['login'], r['name']) for r in github_repos(org)]
    commits = {r: github_commits(*r) for r in repos}
    weekly_commits = collections.defaultdict(int)
    for weeks in commits.values():
        for week in weeks:
            date = datetime.datetime.fromtimestamp(week['week'])
            weekly_commits[date] += week['total']
    print 'Week,commits'
    for week, total in sorted(weekly_commits.items()):
        print '%s,%d' % (week.strftime('%Y-%m-%d'), total)

org_commits(sys.argv[2])
