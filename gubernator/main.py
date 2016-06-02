#!/usr/bin/env python

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

import functools
import json
import logging
import re
import os
import urllib
import zlib

import webapp2
import jinja2

from google.appengine.api import memcache

import defusedxml.ElementTree as ET
import cloudstorage as gcs

import filters

BUCKET_WHITELIST = {
    'kubernetes-jenkins',
    'kube_azure_log',
    'rktnetes-jenkins',
    'kubernetes-github-redhat',
    'kopeio-kubernetes-e2e',
}

DEFAULT_JOBS = {
    'kubernetes-jenkins/logs/': {
        'kubelet-gce-e2e-ci',
        'kubernetes-build',
        'kubernetes-e2e-gce',
        'kubernetes-e2e-gce-scalability',
        'kubernetes-e2e-gce-slow',
        'kubernetes-e2e-gke',
        'kubernetes-e2e-gke-slow',
        'kubernetes-kubemark-5-gce',
        'kubernetes-kubemark-500-gce',
        'kubernetes-test-go',
    }
}

JINJA_ENVIRONMENT = jinja2.Environment(
    loader=jinja2.FileSystemLoader(os.path.dirname(__file__) + '/templates'),
    extensions=['jinja2.ext.autoescape'],
    trim_blocks=True,
    autoescape=True)
JINJA_ENVIRONMENT.line_statement_prefix = '%'
filters.register(JINJA_ENVIRONMENT.filters)


def pad_numbers(s):
    """Modify a string to make its numbers suitable for natural sorting."""
    return re.sub(r'\d+', lambda m: m.group(0).rjust(16, '0'), s)


def memcache_memoize(prefix, expires=60 * 60, neg_expires=60):
    """Decorate a function to memoize its results using memcache.

    The function must take a single string as input, and return a pickleable
    type.

    Args:
        prefix: A prefix for memcache keys to use for memoization.
        expires: How long to memoized values, in seconds.
        neg_expires: How long to memoize falsey values, in seconds
    Returns:
        A decorator closure to wrap the function.
    """
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
                    memcache.add(key, data, expires, namespace=namespace)
                else:
                    memcache.add(key, data, neg_expires, namespace=namespace)
                return data
        return wrapped
    return wrapper


def gcs_read(path):
    """Read a file from GCS. Returns None on errors."""
    try:
        with gcs.open(path) as f:
            return f.read()
    except gcs.errors.Error:
        return None


@memcache_memoize('gs-ls://', expires=60)
def gcs_ls(path):
    """Enumerate files in a GCS directory. Returns a list of FileStats."""
    if path[-1] != '/':
      path += '/'
    return list(gcs.listbucket(path, delimiter='/'))


def parse_junit(xml):
    """Generate failed tests as a series of (name, duration, text) tuples."""
    tree = ET.fromstring(xml)
    if tree.tag == 'testsuite':
        for child in tree:
            name = child.attrib['name']
            time = float(child.attrib['time'])
            for param in child.findall('failure'):
                yield name, time, param.text
    elif tree.tag == 'testsuites':
        for testsuite in tree:
            suite_name = testsuite.attrib['name']
            for child in testsuite.findall('testcase'):
                name = '%s %s' % (suite_name, child.attrib['name'])
                time = float(child.attrib['time'])
                for param in child.findall('failure'):
                    yield name, time, param.text
    else:
        logging.error('unable to find failures, unexpected tag %s', tree.tag)

@memcache_memoize('build-details://', expires=60 * 60 * 4)
def build_details(build_dir):
    """
    Collect information from a build directory.

    Args:
        build_dir: GCS path containing a build's results.
    Returns:
        started: value from started.json {'version': ..., 'timestamp': ...}
        finished: value from finished.json {'timestamp': ..., 'result': ...}
        failures: list of (name, duration, text) tuples
    """
    started = gcs_read(build_dir + '/started.json')
    finished = gcs_read(build_dir + '/finished.json')
    if not (started and finished):
        # TODO: handle builds that have started but not finished properly.
        # Right now they show an empty page (404), but should show the version
        # and when the build started.
        return
    started = json.loads(started)
    finished = json.loads(finished)
    failures = []
    junit_paths = [f.filename for f in gcs_ls('%s/artifacts' % build_dir)
                   if re.match(r'junit_.*\.xml', os.path.basename(f.filename))]
    for junit_path in junit_paths:
        junit = gcs_read(junit_path)
        if junit is None:
            continue
        failures.extend(parse_junit(decompress(junit)))
    return started, finished, failures


def decompress(data):
    """Decompress data if GZIP-compressed, but pass normal data thorugh."""
    if data.startswith('\x1f\x8b'):  # gzip magic
        return zlib.decompress(data, 15 | 16)
    return data


class RenderingHandler(webapp2.RequestHandler):
    """Base class for Handlers that render Jinja templates."""
    def render(self, template, context):
        """Render a context dictionary using a given template."""
        template = JINJA_ENVIRONMENT.get_template(template)
        self.response.write(template.render(context))

    def check_bucket(self, prefix):
        if prefix in BUCKET_WHITELIST:
            return
        if prefix[:prefix.find('/')] not in BUCKET_WHITELIST:
            self.abort(404)


class IndexHandler(RenderingHandler):
    """Render the index."""
    def get(self):
        self.render("index.html", {'jobs': DEFAULT_JOBS})


class BuildHandler(RenderingHandler):
    """Show information about a Build and its failing tests."""
    def get(self, prefix, job, build):
        self.check_bucket(prefix)
        job_dir = '/%s/%s/' % (prefix, job)
        build_dir = job_dir + build
        details = build_details(build_dir)
        if not details:
            self.response.write("Unable to load build details from %s"
                                % job_dir)
            self.error(404)
            return
        started, finished, failures = details
        commit = started['version'].split('+')[-1]
        self.render('build.html', dict(
            job_dir=job_dir, build_dir=build_dir, job=job, build=build,
            commit=commit, started=started, finished=finished,
            failures=failures))


class BuildListHandler(RenderingHandler):
    """Show a list of Builds for a Job."""
    def get(self, prefix, job):
        self.check_bucket(prefix)
        job_dir = '/%s/%s/' % (prefix, job)
        fstats = gcs_ls(job_dir)
        fstats.sort(key=lambda f: pad_numbers(f.filename), reverse=True)
        self.render('build_list.html',
                    dict(job=job, job_dir=job_dir, fstats=fstats))


class JobListHandler(RenderingHandler):
    """Show a list of Jobs in a directory."""
    def get(self, prefix):
        self.check_bucket(prefix)
        jobs_dir = '/%s' % prefix
        fstats = gcs_ls(jobs_dir)
        fstats.sort()
        self.render('job_list.html', dict(jobs_dir=jobs_dir, fstats=fstats))


app = webapp2.WSGIApplication([
    (r'/', IndexHandler),
    (r'/jobs/(.*)$', JobListHandler),
    (r'/builds/(.*)/([^/]+)/?', BuildListHandler),
    (r'/build/(.*)/([^/]+)/(\d+)/?', BuildHandler),
], debug=True)

if os.environ.get('SERVER_SOFTWARE','').startswith('Development'):
    # inject some test data so there's a page with some content
    import tarfile
    tf = tarfile.open('test_data/kube_results.tar.gz')
    prefix = '/kubernetes-jenkins/logs/kubernetes-soak-continuous-e2e-gce/'
    for member in tf:
        if member.isfile():
            with gcs.open(prefix + member.name, 'w') as f:
                f.write(tf.extractfile(member).read())
    with gcs.open(prefix + '5044/started.json', 'w') as f:
        f.write('test')  # So JobHandler has more than one item.
