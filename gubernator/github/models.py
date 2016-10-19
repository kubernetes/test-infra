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

import datetime

import google.appengine.ext.ndb as ndb


class GithubResource(ndb.Model):
    # A key holder used to define an entitygroup for
    # each Issue/PR, for easy ancestor queries.
    @staticmethod
    def make_key(repo, number):
        return ndb.Key(GithubResource, '%s %s' % (repo, number))


class GithubWebhookRaw(ndb.Model):
    repo = ndb.StringProperty()
    number = ndb.IntegerProperty(indexed=False)
    event = ndb.StringProperty()
    timestamp = ndb.DateTimeProperty(auto_now_add=True)
    body = ndb.TextProperty(compressed=True)


def from_iso8601(t):
    return t and datetime.datetime.strptime(t, '%Y-%m-%dT%H:%M:%SZ')


def make_kwargs(body, fields):
    kwargs = {}
    for field in fields:
        if field.endswith('_at'):
            kwargs[field] = from_iso8601(body[field])
        else:
            kwargs[field] = body[field]
    return kwargs


class GHStatus(ndb.Model):
    # Key: {repo}\t{sha}\t{context}
    state = ndb.StringProperty(indexed=False)
    target_url = ndb.StringProperty(indexed=False)
    description = ndb.TextProperty()

    created_at = ndb.DateTimeProperty(indexed=False)
    updated_at = ndb.DateTimeProperty(indexed=False)


    @staticmethod
    def make_key(repo, sha, context):
        return ndb.Key(GHStatus, '%s\t%s\t%s' % (repo, sha, context))

    @staticmethod
    def make(repo, sha, context, **kwargs):
        return GHStatus(key=GHStatus.make_key(repo, sha, context), **kwargs)

    @staticmethod
    def query_for_sha(repo, sha):
        before = GHStatus.make_key(repo, sha, '')
        after = GHStatus.make_key(repo, sha, '\x7f')
        return GHStatus.query(GHStatus.key > before, GHStatus.key < after)

    @staticmethod
    def from_json(body):
        kwargs = make_kwargs(body,
            'sha context state target_url description '
            'created_at updated_at'.split())
        kwargs['repo'] = body['name']
        return GHStatus.make(**kwargs)

    @property
    def repo(self):
        return self.key.id().split('\t', 1)[0]

    @property
    def sha(self):
        return self.key.id().split('\t', 2)[1]

    @property
    def context(self):
        return self.key.id().split('\t', 2)[2]


class GHIssueDigest(ndb.Model):
    # Key: {repo} {number}
    is_pr = ndb.BooleanProperty()
    is_open = ndb.BooleanProperty()
    involved = ndb.StringProperty(repeated=True)
    xref = ndb.StringProperty(repeated=True)
    payload = ndb.JsonProperty()
    updated_at = ndb.DateTimeProperty()
    head = ndb.StringProperty()

    @staticmethod
    def make_key(repo, number):
        return ndb.Key(GHIssueDigest, '%s %s' % (repo, number))

    @staticmethod
    def make(repo, number, is_pr, is_open, involved, payload, updated_at):
        return GHIssueDigest(key=GHIssueDigest.make_key(repo, number),
            is_pr=is_pr, is_open=is_open, involved=involved, payload=payload,
            updated_at=updated_at, head=payload.get('head'),
            xref=payload.get('xrefs', []))

    @staticmethod
    def get(repo, number):
        return GHIssueDigest.make_key(repo, number).get()

    @property
    def repo(self):
        return self.key.id().split()[0]

    @property
    def number(self):
        return int(self.key.id().split()[1])

    @staticmethod
    def find_head(repo, head):
        return GHIssueDigest.query(GHIssueDigest.key > GHIssueDigest.make_key(repo, ''),
                                   GHIssueDigest.key < GHIssueDigest.make_key(repo, '~'),
                                   GHIssueDigest.head == head)

    @staticmethod
    def find_xrefs(xref):
        return GHIssueDigest.query(GHIssueDigest.xref == xref)


@ndb.transactional
def save_if_newer(obj):
    assert obj.updated_at is not None
    old = obj.key.get()
    if old is None:
        obj.put()
        return True
    else:
        if old.updated_at is None or obj.updated_at >= old.updated_at:
            obj.put()
            return True
        return False
