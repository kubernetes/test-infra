#!/usr/bin/env python
#
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

import datetime
import functools
import hashlib
import json
import logging
import re
import os
import urllib
import zlib

import webapp2
import jinja2

from google.appengine.api import memcache

import lib.defusedxml.ElementTree as ET
import lib.cloudstorage as gcs

BUCKET_WHITELIST = {
    'kubernetes-jenkins',
}

GITHUB_VIEW_TEMPLATE = 'https://github.com/kubernetes/kubernetes/blob/%s/%s#L%s'

JINJA_ENVIRONMENT = jinja2.Environment(
    loader=jinja2.FileSystemLoader(os.path.dirname(__file__) + '/templates'),
    extensions=['jinja2.ext.autoescape'],
    trim_blocks=True,
    autoescape=True)


def jinja_filter(func):
    JINJA_ENVIRONMENT.filters[func.__name__.replace('format_', '')] = func
    return func


@jinja_filter
def format_timestamp(ts):
    t = datetime.datetime.fromtimestamp(ts)
    return t.strftime('%F %H:%M')


@jinja_filter
def format_duration(seconds):
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


@jinja_filter
def slugify(inp):
    inp = re.sub(r'[^\w\s-]+', '', inp)
    return re.sub(r'\s+', '-', inp).lower()


@jinja_filter
def linkify_stacktrace(inp, commit):
    inp = str(jinja2.escape(inp))
    if not commit:
        return inp
    def rep(m):
        path, line = m.groups()
        return '<a href="%s">%s</a>' % (
            GITHUB_VIEW_TEMPLATE % (commit, path, line), m.group(0))
    return jinja2.Markup(re.sub(r'^/\S*/kubernetes/(\S+):(\d+)$', rep, inp, 
                                flags=re.MULTILINE))


def memcache_memoize(prefix, exp=60 * 60, neg_exp=60):
    # setting the namespace based on the current version prevents different
    # versions from sharing cache values -- meaning there's no need to worry
    # about incompatible old key/value pairs
    namespace = os.environ['CURRENT_VERSION_ID']
    def wrapper(func):
        @functools.wraps(func)
        def wrapped(arg):
            key = prefix + arg
            data = memcache.get(key, namespace=namespace)
            if data is not None:
                return data
            else:
                data = func(arg)
                if data:
                    memcache.add(key, data, exp, namespace=namespace)
                else:
                    memcache.add(key, data, neg_exp, namespace=namespace)
                return data
        return wrapped
    return wrapper


@memcache_memoize('gs://')
def gcs_read(path):
    try:
        with gcs.open(path) as f:
            return f.read()
    except gcs.errors.Error:
        return None


@memcache_memoize('build-details://', exp=60 * 60 * 4)
def build_details(build_dir):
    started = gcs_read(build_dir + '/started.json')
    finished = gcs_read(build_dir + '/finished.json')
    if not (started and finished):
        return
    started = json.loads(started)
    finished = json.loads(finished)
    failures = []
    for n in xrange(1, 99):
        junit = gcs_read(
            '%s/artifacts/junit_%02d.xml' % (build_dir, n))
        if junit is None:
            break
        failures.extend(parse_junit(decompress(junit)))
    return started, finished, failures


def decompress(data):
    if data.startswith('\x1f\x8b'):  # gzip magic
        return zlib.decompress(data, 15 | 16)
    return data


def parse_junit(xml):
    for child in ET.fromstring(xml):
        name = child.attrib['name']
        time = float(child.attrib['time'])
        failed = False
        skipped = False
        text = None
        for param in child:
            if param.tag == 'skipped':
                skipped = True
                text = param.text
            elif param.tag == 'failure':
                failed = True
                text = param.text
        if failed:
            yield name, time, text


class BuildHandler(webapp2.RequestHandler):
    def get(self, bucket, prefix, job, build):
        if bucket not in BUCKET_WHITELIST:
            self.error(404)
            return
        build_dir = '/%s/%s%s/%s' % (bucket, prefix, job, build)
        details = build_details(build_dir)
        if not details:
            self.error(404)
            return
        started, finished, failures = details
        commit = started['version'].split('+')[1]
        template = JINJA_ENVIRONMENT.get_template('build.html')
        self.response.write(template.render(dict(
            build_dir=build_dir, job=job, build=build, commit=commit,
            started=started, finished=finished, failures=failures)))

app = webapp2.WSGIApplication([
    (r'/build/([-\w]+)/([-\w/]*/)?([-\w]+)/(\d+)/?', BuildHandler),
    # webapp2.Route('/upload', gcs_upload.Upload, 'upload'),
], debug=True)

if os.environ.get('SERVER_SOFTWARE','').startswith('Development'):
    # inject some test data so there's a page with some content
    import tarfile
    tf = tarfile.open('kube_results.tar.gz')
    prefix = '/kubernetes-jenkins/logs/kubernetes-soak-continuous-e2e-gce/'
    for member in tf:
        if member.isfile():
            with gcs.open(prefix + member.name, 'w') as f:
                f.write(tf.extractfile(member).read())
