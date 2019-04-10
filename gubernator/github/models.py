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

import logging
import datetime
import json

import google.appengine.ext.ndb as ndb


class GithubResource(ndb.Model):
    # A key holder used to define an entitygroup for
    # each Issue/PR, for easy ancestor queries.
    @staticmethod
    def make_key(repo, number):
        return ndb.Key(GithubResource, '%s %s' % (repo, number))


def shrink(body):
    """Recursively remove Github API urls from an object to make it more human-readable."""
    toremove = []
    for key, value in body.iteritems():
        if isinstance(value, basestring):
            if key.endswith('url'):
                if (value.startswith('https://api.github.com/') or
                    value.startswith('https://avatars.githubusercontent.com')):
                    toremove.append(key)
        elif isinstance(value, dict):
            shrink(value)
        elif isinstance(value, list):
            for el in value:
                if isinstance(el, dict):
                    shrink(el)
    for key in toremove:
        body.pop(key)
    return body


class GithubWebhookRaw(ndb.Model):
    repo = ndb.StringProperty()
    number = ndb.IntegerProperty(indexed=False)
    event = ndb.StringProperty()
    guid = ndb.StringProperty()
    timestamp = ndb.DateTimeProperty(auto_now_add=True)
    body = ndb.TextProperty(compressed=True)

    def to_tuple(self):
        return (self.event, shrink(json.loads(self.body)), float(self.timestamp.strftime('%s.%f')))


def from_iso8601(t):
    if not t:
        return t
    if t.endswith('Z'):
        return datetime.datetime.strptime(t, '%Y-%m-%dT%H:%M:%SZ')
    elif t.endswith('+00:00'):
        return datetime.datetime.strptime(t, '%Y-%m-%dT%H:%M:%S+00:00')
    else:
        logging.warning('unparseable time value: %s', t)
        return None


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

    @property
    def url(self):
        return 'https://github.com/%s/issues/%s' % tuple(self.key.id().split())

    @property
    def title(self):
        return self.payload.get('title', '')

    @staticmethod
    def find_head(repo, head):
        return GHIssueDigest.query(GHIssueDigest.key > GHIssueDigest.make_key(repo, ''),
                                   GHIssueDigest.key < GHIssueDigest.make_key(repo, '~'),
                                   GHIssueDigest.head == head)

    @staticmethod
    @ndb.tasklet
    def find_xrefs_async(xref):
        issues = yield GHIssueDigest.query(GHIssueDigest.xref == xref).fetch_async()
        raise ndb.Return(list(issues))

    @staticmethod
    @ndb.tasklet
    def find_xrefs_multi_async(xrefs):
        """
        Given a list of xrefs to search for, return a dict of lists
        of result values. Xrefs that have no corresponding issues are
        not represented in the dictionary.
        """
        # The IN operator does multiple sequential queries and ORs them
        # together. This is slow here-- a range query is faster, since
        # this is used to get xrefs for a set of contiguous builds.
        if not xrefs:  # nothing => nothing
            raise ndb.Return({})
        xrefs = set(xrefs)
        issues = yield GHIssueDigest.query(
            GHIssueDigest.xref >= min(xrefs),
            GHIssueDigest.xref <= max(xrefs)).fetch_async(batch_size=500)
        refs = {}
        for issue in issues:
            for xref in issue.xref:
                if xref in xrefs:
                    refs.setdefault(xref, []).append(issue)
        raise ndb.Return(refs)

    @staticmethod
    def find_open_prs():
        # pylint: disable=singleton-comparison
        return GHIssueDigest.query(GHIssueDigest.is_pr == True,
                                   GHIssueDigest.is_open == True)

    @staticmethod
    def find_open_prs_for_repo(repo):
        return (GHIssueDigest.find_open_prs()
            .filter(GHIssueDigest.key > GHIssueDigest.make_key(repo, ''),
                    GHIssueDigest.key < GHIssueDigest.make_key(repo, '~')))


class GHUserState(ndb.Model):
    # Key: {github username}
    acks = ndb.JsonProperty()  # dict of issue keys => ack time (seconds since epoch)

    @staticmethod
    def make_key(user):
        return ndb.Key(GHUserState, user)

    @staticmethod
    def make(user, acks=None):
        return GHUserState(key=GHUserState.make_key(user), acks=acks or {})


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
