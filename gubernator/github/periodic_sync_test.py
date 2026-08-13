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

# pylint: disable=no-self-use

import cPickle as pickle
import json

import webtest

from google.appengine.ext import deferred
from google.appengine.ext import testbed

import main_test
import models
import periodic_sync
import secrets

app = webtest.TestApp(periodic_sync.app)

TOKEN = 'gh_auth_secret'

def pr_data(number):
    return {
        'number': number,
        'state': 'open',
        'user': {'login': 'a'},
        'assignees': [{'login': 'b'}],
        'title': 'some fix %d' % number,
        'head': {'sha': 'abcdef'},
    }

class SyncTest(main_test.TestBase):
    def setUp(self):
        self.init_stubs()
        self.taskqueue = self.testbed.get_stub(testbed.TASKQUEUE_SERVICE_NAME)
        secrets.put('github_token', TOKEN, per_host=False)
        self.gh_data = {}

    def inject_pr(self, repo, number):
        models.GHIssueDigest.make(
            repo, number, True, True, [],
            {'author': 'someone', 'assignees': [], 'title': '#%d' % number},
            None).put()

    def get_deferred_funcs(self):
        out = []
        for task in self.taskqueue.get_filtered_tasks():
            func, args, _kwargs = pickle.loads(task.payload)
            out.append((func.__name__, args))
        return out

    def test_repo_dispatch(self):
        self.inject_pr('a/b', 3)
        self.inject_pr('a/b', 4)
        self.inject_pr('k/t', 90)

        app.get('/sync')

        self.assertEqual(self.get_deferred_funcs(),
            [('sync_repo', (TOKEN, 'a/b')),
             ('sync_repo', (TOKEN, 'k/t'))])

    def urlfetch_github_stub(self, url, _payload, method,
                             headers, _request, response, **_kwargs):
        assert method == 'GET'
        assert headers[0].key() == 'Authorization'
        assert headers[0].value() == 'token ' + TOKEN
        path = url[url.find('.com')+4:]
        content, response_headers = self.gh_data[path]
        response.set_content(json.dumps(content))
        response.set_statuscode(200)
        for k, v in response_headers.iteritems():
            header = response.add_header()
            header.set_key(k)
            header.set_value(v)

    def init_github_fake(self):
        # Theoretically, I should be able to pass this to init_urlfetch_stub.
        # Practically, that doesn't work for unknown reasons, and this is fine.
        uf = self.testbed.get_stub('urlfetch')
        uf._urlmatchers_to_fetch_functions.append(  # pylint: disable=protected-access
            (lambda u: u.startswith('https://api.github.com'), self.urlfetch_github_stub))

    def test_get_prs_from_github(self):
        self.init_github_fake()

        base_url = '/repos/a/b/pulls?state=open&per_page=100'
        def make_headers(page):
            frag = '<https://api.github.com%s' % base_url
            link = '%s&page=4>; rel="last"' % frag
            if page < 4:
                link = '%s&page=%d>; rel="next", %s' % (frag, page + 1, link)
            if page > 1:
                link = '%s&page=%d>; rel="prev", %s' % (frag, page - 1, link)
            print link
            return {'Link': link}

        prs = [pr_data(n) for n in range(400)]
        self.gh_data = {
            base_url:             (prs[:100], make_headers(1)),
            base_url + '&page=2': (prs[100:200], make_headers(2)),
            base_url + '&page=3': (prs[200:300], make_headers(3)),
            base_url + '&page=4': (prs[300:], make_headers(4)),
        }

        got_prs = periodic_sync.get_prs_from_github(TOKEN, 'a/b')
        got_pr_nos = [pr['number'] for pr in got_prs]
        expected_pr_nos = [pr['number'] for pr in prs]

        self.assertEqual(got_pr_nos, expected_pr_nos)

    def test_sync_repo(self):
        self.init_github_fake()
        self.gh_data = {
            '/repos/a/b/pulls?state=open&per_page=100':
                ([pr_data(11), pr_data(12), pr_data(13)], {}),
        }

        # PRs <10 are closed
        # PRs >10 are open
        self.inject_pr('a/b', 1)
        self.inject_pr('a/b', 2)
        self.inject_pr('a/b', 3)
        self.inject_pr('a/b', 11)
        self.inject_pr('a/b', 12)

        periodic_sync.sync_repo(TOKEN, 'a/b')

        for task in self.taskqueue.get_filtered_tasks():
            deferred.run(task.payload)

        open_prs = models.GHIssueDigest.find_open_prs().fetch()
        open_prs = [pr.number for pr in open_prs]

        self.assertEqual(open_prs, [11, 12, 13])
