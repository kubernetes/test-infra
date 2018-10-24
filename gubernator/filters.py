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
import hashlib
import os
import re
import time
import urllib
import urlparse

import jinja2


GITHUB_VIEW_TEMPLATE = 'https://github.com/%s/blob/%s/%s#L%s'
GITHUB_COMMIT_TEMPLATE = 'https://github.com/%s/commit/%s'
LINKIFY_RE = re.compile(
    r'(^\s*/\S*/)(kubernetes/(\S+):(\d+)(?: \+0x[0-9a-f]+)?)$',
    flags=re.MULTILINE)


def do_timestamp(unix_time, css_class='timestamp', tmpl='%F %H:%M'):
    """Convert an int Unix timestamp into a human-readable datetime."""
    t = datetime.datetime.utcfromtimestamp(unix_time)
    return jinja2.Markup('<span class="%s" data-epoch="%s">%s</span>' %
                         (css_class, unix_time, t.strftime(tmpl)))


def do_dt_to_epoch(dt):
    return time.mktime(dt.timetuple())


def do_shorttimestamp(unix_time):
    t = datetime.datetime.utcfromtimestamp(unix_time)
    return jinja2.Markup('<span class="shorttimestamp" data-epoch="%s">%s</span>' %
                         (unix_time, t.strftime('%m-%d %H:%M')))


def do_duration(seconds):
    """Convert a numeric duration in seconds into a human-readable string."""
    hours, seconds = divmod(seconds, 3600)
    minutes, seconds = divmod(seconds, 60)
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


def do_linkify_stacktrace(inp, commit, repo):
    """Add links to a source code viewer for every mentioned source line."""
    inp = unicode(jinja2.escape(inp))
    if not commit:
        return jinja2.Markup(inp)  # this was already escaped, mark it safe!
    def rep(m):
        prefix, full, path, line = m.groups()
        return '%s<a href="%s">%s</a>' % (
            prefix,
            GITHUB_VIEW_TEMPLATE % (repo, commit, path, line),
            full)
    return jinja2.Markup(LINKIFY_RE.sub(rep, inp))


def do_github_commit_link(commit, repo):
    commit_url = jinja2.escape(GITHUB_COMMIT_TEMPLATE % (repo, commit))
    return jinja2.Markup('<a href="%s">%s</a>' % (commit_url, commit[:8]))


def do_maybe_linkify(inp):
    try:
        if urlparse.urlparse(inp).scheme in ('http', 'https'):
            inp = unicode(jinja2.escape(inp))
            return jinja2.Markup('<a href="%s">%s</a>' % (inp, inp))
    except (AttributeError, TypeError):
        pass
    return inp


def do_testcmd(name):
    if name.startswith('k8s.io/'):
        try:
            pkg, name = name.split(' ')
        except ValueError:  # don't block the page render
            logging.error('Unexpected Go unit test name %r', name)
            return name
        return 'go test -v %s -run %s$' % (pkg, name)
    elif name.startswith('istio.io/'):
        return ''
    elif name.startswith('//'):
        return 'bazel test %s' % name
    elif name.startswith('verify '):
        return 'make verify WHAT=%s' % name.split(' ')[1]
    else:
        name = re.sub(r'^\[k8s\.io\] ', '', name)
        name_escaped = re.escape(name).replace('\\ ', '\\s')

        test_args = ('--ginkgo.focus=%s$' % name_escaped)
        return "go run hack/e2e.go -v --test --test_args='%s'" % test_args


def do_parse_pod_name(text):
    """Find the pod name from the failure and return the pod name."""
    p = re.search(r' pod (\S+)', text)
    if p:
        return re.sub(r'[\'"\\:]', '', p.group(1))
    else:
        return ""


def do_label_attr(labels, name):
    """
    >> do_label_attr(['needs-rebase', 'size/XS'], 'size')
    'XS'
    """
    name += '/'
    for label in labels:
        if label.startswith(name):
            return label[len(name):]
    return ''

def do_classify_size(payload):
    """
    Determine the size class for a PR, based on either its labels or
    on the magnitude of its changes.
    """
    size = do_label_attr(payload['labels'], 'size')
    if not size and 'additions' in payload and 'deletions' in payload:
        lines = payload['additions'] + payload['deletions']
        # based on mungegithub/mungers/size.go
        for limit, label in [
            (10, 'XS'),
            (30, 'S'),
            (100, 'M'),
            (500, 'L'),
            (1000, 'XL')
        ]:
            if lines < limit:
                return label
        return 'XXL'
    return size


def has_lgtm_without_missing_approval(payload, user):
    labels = payload.get('labels', []) or []
    return 'lgtm' in labels and not (
        user in payload.get('approvers', [])
        and 'approved' not in labels)


def do_render_status(payload, user):
    states = set()

    text = 'Pending'
    if has_lgtm_without_missing_approval(payload, user):
        text = 'LGTM'
    elif user in payload.get('attn', {}):
        text = payload['attn'][user].title()
        if '#' in text:  # strip start/end attn timestamps
            text = text[:text.index('#')]

    for ctx, (state, _url, desc) in payload.get('status', {}).items():
        if ctx == 'Submit Queue' and state == 'pending':
            if 'does not have lgtm' in desc.lower():
                # Don't show overall status as pending when Submit
                # won't continue without LGTM.
                continue
        if ctx == 'tide' and state == 'pending':
            # Ignore pending tide statuses for now.
            continue
        if ctx == 'code-review/reviewable' and state == 'pending':
            # Reviewable isn't a CI, so we don't care if it's pending.
            # Its dashboard might replace all of this eventually.
            continue
        states.add(state)

    icon = ''
    title = ''
    if 'failure' in states:
        icon = 'x'
        state = 'failure'
        title = 'failing tests'
    elif 'pending' in states:
        icon = 'primitive-dot'
        state = 'pending'
        title = 'pending tests'
    elif 'success' in states:
        icon = 'check'
        state = 'success'
        title = 'tests passing'
    if icon:
        icon = '<span class="text-%s octicon octicon-%s" title="%s"></span>' % (
            state, icon, title)
    return jinja2.Markup('%s%s' % (icon, text))


def do_get_latest(payload, user):
    text = payload.get('attn', {}).get(user)
    if not text:
        return None
    if '#' not in text:
        return None
    _text, _start, latest = text.rsplit('#', 2)
    return float(latest)


def do_ltrim(s, needle):
    if s.startswith(needle):
        return s[len(needle):]
    return s


def do_select(seq, pred):
    return filter(pred, seq)


def do_tg_url(testgrid_query, test_name=''):
    if test_name:
        regex = '^Overall$|' + re.escape(test_name)
        testgrid_query += '&include-filter-by-regex=%s' % urllib.quote(regex)
    return 'https://testgrid.k8s.io/%s' % testgrid_query


def do_gcs_browse_url(gcs_path):
    if not gcs_path.endswith('/'):
        gcs_path += '/'
    return 'http://gcsweb.k8s.io/gcs' + gcs_path


static_hashes = {}

def do_static(filename):
    filename = 'static/%s' % filename
    if filename not in static_hashes:
        data = open(filename).read()
        static_hashes[filename] = hashlib.sha1(data).hexdigest()[:10]
    return '/%s?%s' % (filename, static_hashes[filename])


do_basename = os.path.basename
do_dirname = os.path.dirname
do_quote_plus = urllib.quote_plus


def register(filters):
    """Register do_* functions in this module in a dictionary."""
    for name, func in globals().items():
        if name.startswith('do_'):
            filters[name[3:]] = func
