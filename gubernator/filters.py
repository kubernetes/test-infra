# Copyright 2016 The Kubernetes Authors All rights reserved.
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
import os
import re

import jinja2


GITHUB_VIEW_TEMPLATE = 'https://github.com/kubernetes/kubernetes/blob/%s/%s#L%s'
GITHUB_COMMIT_TEMPLATE = 'https://github.com/kubernetes/kubernetes/commit/%s'



def do_timestamp(unix_time):
    """Convert an int Unix timestamp into a human-readable datetime."""
    t = datetime.datetime.utcfromtimestamp(unix_time)
    return jinja2.Markup('<span class="timestamp" data-epoch="%s">%s</span>' %
                         (unix_time, t.strftime('%F %H:%M')))


def do_shorttimestamp(unix_time):
    t = datetime.datetime.utcfromtimestamp(unix_time)
    return jinja2.Markup('<span class="shorttimestamp" data-epoch="%s">%s</span>' %
                         (unix_time, t.strftime('%d %H:%M')))


def do_duration(seconds):
    """Convert a numeric duration in seconds into a human-readable string."""
    hours, seconds = divmod(seconds, 3600)
    minutes, seconds = divmod(seconds, 60)
    out = ''
    if hours:
        return '%dh%dm' % (hours, minutes)
    if minutes:
        return '%dm%ds' % (minutes, seconds)
    else:
        if seconds < 10:
            return '%.2fs' % seconds
        return '%ds' % seconds


def do_slugify(inp):
    """Convert an arbitrary string into a url-safe slug."""
    inp = re.sub(r'[^\w\s-]+', '', inp)
    return re.sub(r'\s+', '-', inp).lower()


def do_linkify_stacktrace(inp, commit):
    """Add links to a source code viewer for every mentioned source line."""
    inp = unicode(jinja2.escape(inp))
    if not commit:
        return jinja2.Markup(inp)  # this was already escaped, mark it safe!
    def rep(m):
        path, line = m.groups()
        return '<a href="%s">%s</a>' % (
            GITHUB_VIEW_TEMPLATE % (commit, path, line), m.group(0))
    return jinja2.Markup(re.sub(r'^/\S*/kubernetes/(\S+):(\d+)$', rep, inp,
                                flags=re.MULTILINE))


def do_github_commit_link(commit):
    commit_url = jinja2.escape(GITHUB_COMMIT_TEMPLATE % commit)
    return jinja2.Markup('<a href="%s">%s</a>' % (commit_url, commit[:6]))


def do_testcmd(name):
    if name.startswith('k8s.io/'):
        try:
            pkg, name = name.split(' ')
        except ValueError:  # don't block the page render
            logging.error('Unexpected Go unit test name %r', name)
            return name
        return 'go test -v %s -run %s$' % (pkg, name)
    else:
        name = re.sub(r'^\[k8s\.io\] ', '', name)
        name_escaped = re.escape(name).replace('\\ ', '\\s')

        test_args = ('--ginkgo.focus=%s$' % name_escaped)
        return "go run hack/e2e.go -v -test --test_args='%s'" % test_args

do_basename = os.path.basename
do_dirname = os.path.dirname


def register(filters):
    """Register do_* functions in this module in a dictionary."""
    for name, func in globals().items():
        if name.startswith('do_'):
            filters[name[3:]] = func
