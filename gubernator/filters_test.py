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

import re
import unittest
import urllib

import filters

import jinja2


def linkify(inp, commit):
    return str(filters.do_linkify_stacktrace(
        inp, commit, 'kubernetes/kubernetes'))


class HelperTest(unittest.TestCase):
    def test_timestamp(self):
        self.assertEqual(
            '<span class="timestamp" data-epoch="1461100940">'
            '2016-04-19 21:22</span>',
            filters.do_timestamp(1461100940))

    def test_duration(self):
        for duration, expected in {
            3.56: '3.56s',
            13.6: '13s',
            78.2: '1m18s',
            60 * 62 + 3: '1h2m',
        }.iteritems():
            self.assertEqual(expected, filters.do_duration(duration))

    def test_linkify_safe(self):
        self.assertEqual('&lt;a&gt;',
                         linkify('<a>', '3'))

    def test_linkify(self):
        linked = linkify(
            "/go/src/k8s.io/kubernetes/test/example.go:123", 'VERSION')
        self.assertIn('<a href="https://github.com/kubernetes/kubernetes/blob/'
                      'VERSION/test/example.go#L123">', linked)

    def test_linkify_trailing(self):
        linked = linkify(
            "    /go/src/k8s.io/kubernetes/test/example.go:123 +0x1ad", 'VERSION')
        self.assertIn('github.com', linked)

    def test_linkify_unicode(self):
        # Check that Unicode characters pass through cleanly.
        linked = filters.do_linkify_stacktrace(u'\u883c', 'VERSION', '')
        self.assertEqual(linked, u'\u883c')

    def test_maybe_linkify(self):
        for inp, expected in [
            (3, 3),
            ({"a": "b"}, {"a": "b"}),
            ("", ""),
            ("whatever", "whatever"),
            ("http://example.com",
             jinja2.Markup('<a href="http://example.com">http://example.com</a>')),
            ("http://&",
             jinja2.Markup('<a href="http://&amp;">http://&amp;</a>')),
        ]:
            self.assertEqual(filters.do_maybe_linkify(inp), expected)

    def test_slugify(self):
        self.assertEqual('k8s-test-foo', filters.do_slugify('[k8s] Test Foo'))

    def test_testcmd(self):
        for name, expected in (
            ('k8s.io/kubernetes/pkg/api/errors TestErrorNew',
             'go test -v k8s.io/kubernetes/pkg/api/errors -run TestErrorNew$'),
            ('[k8s.io] Proxy [k8s.io] works',
            "go run hack/e2e.go -v --test --test_args='--ginkgo.focus="
            "Proxy\\s\\[k8s\\.io\\]\\sworks$'"),
            ('//pkg/foo/bar:go_default_test',
            'bazel test //pkg/foo/bar:go_default_test'),
            ('verify typecheck', 'make verify WHAT=typecheck')):
            print 'test name:', name
            self.assertEqual(filters.do_testcmd(name), expected)

    def test_classify_size(self):
        self.assertEqual(filters.do_classify_size(
            {'labels': {'size/FOO': 1}}), 'FOO')
        self.assertEqual(filters.do_classify_size(
            {'labels': {}, 'additions': 70, 'deletions': 20}), 'M')

    def test_render_status_basic(self):
        payload = {'status': {'ci': ['pending', '', '']}}
        self.assertEqual(str(filters.do_render_status(payload, '')),
            '<span class="text-pending octicon octicon-primitive-dot" title="pending tests">'
            '</span>Pending')

    def test_render_status_complex(self):
        def expect(payload, expected, user=''):
            # strip the excess html from the result down to the text class,
            # the opticon class, and the rendered text
            result = str(filters.do_render_status(payload, user))
            result = re.sub(r'<span class="text-|octicon octicon-| title="[^"]*"|</span>',
                            '', result)
            result = result.replace('">', ' ')
            self.assertEqual(result, expected)

        statuses = lambda *xs: {str(n): [x, '', ''] for n, x in enumerate(xs)}
        expect({'status': {}}, 'Pending')
        expect({'status': statuses('pending')}, 'pending primitive-dot Pending')
        expect({'status': statuses('failure')}, 'failure x Pending')
        expect({'status': statuses('success')}, 'success check Pending')
        expect({'status': statuses('pending', 'success')}, 'pending primitive-dot Pending')
        expect({'status': statuses('failure', 'pending', 'success')}, 'failure x Pending')

        expect({'status': {'ci': ['success', '', ''],
            'Submit Queue': ['pending', '', 'does not have LGTM']}}, 'success check Pending')
        expect({'status': {'ci': ['success', '', ''],
            'tide': ['pending', '', '']}}, 'success check Pending')
        expect({'status': {'ci': ['success', '', ''],
            'code-review/reviewable': ['pending', '', '10 files left']}}, 'success check Pending')
        expect({'status': {'ci': ['success', '', '']}, 'labels': ['lgtm']}, 'success check LGTM')
        expect({'attn': {'foo': 'Needs Rebase'}}, 'Needs Rebase', user='foo')
        expect({'attn': {'foo': 'Needs Rebase'}, 'labels': {'lgtm'}}, 'LGTM', user='foo')

        expect({'author': 'u', 'labels': ['lgtm']}, 'LGTM', 'u')
        expect({'author': 'b', 'labels': ['lgtm'], 'approvers': ['u'],
                'attn': {'u': 'needs approval'}},
               'Needs Approval', 'u')

    def test_tg_url(self):
        self.assertEqual(
            filters.do_tg_url('a#b'),
            'https://testgrid.k8s.io/a#b')
        self.assertEqual(
            filters.do_tg_url('a#b', '[low] test'),
            'https://testgrid.k8s.io/a#b&include-filter-by-regex=%s' %
            urllib.quote('^Overall$|\\[low\\]\\ test'))

    def test_gcs_browse_url(self):
        self.assertEqual(
            filters.do_gcs_browse_url('/k8s/foo'),
            'http://gcsweb.k8s.io/gcs/k8s/foo/')
        self.assertEqual(
            filters.do_gcs_browse_url('/k8s/bar/'),
            'http://gcsweb.k8s.io/gcs/k8s/bar/')

    def test_pod_name(self):
        self.assertEqual(filters.do_parse_pod_name("start pod 'client-c6671' to"), 'client-c6671')
        self.assertEqual(filters.do_parse_pod_name('tripod "blah"'), '')

        # exercise pathological case
        self.assertEqual(filters.do_parse_pod_name('abcd pode ' * 10000), '')


if __name__ == '__main__':
    unittest.main()
