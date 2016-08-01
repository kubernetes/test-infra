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

import webapp2
import jinja2
import yaml

from google.appengine.api import memcache, urlfetch

import defusedxml.ElementTree as ET
import cloudstorage as gcs

import github.models as ghm
import gcs_async
import filters as jinja_filters
import log_parser
import kubelet_parser
import pull_request
import regex

BUCKET_WHITELIST = {
    re.match(r'gs://([^/]+)', path).group(1)
    for path in yaml.load(open("buckets.yaml"))
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

PR_PREFIX = 'kubernetes-jenkins/pr-logs/pull'

JINJA_ENVIRONMENT = jinja2.Environment(
    loader=jinja2.FileSystemLoader(os.path.dirname(__file__) + '/templates'),
    extensions=['jinja2.ext.autoescape'],
    trim_blocks=True,
    autoescape=True)
JINJA_ENVIRONMENT.line_statement_prefix = '%'
jinja_filters.register(JINJA_ENVIRONMENT.filters)


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
            key = '%s%s' % (prefix, arg)
            data = memcache.get(key, namespace=namespace)
            if data is not None:
                return data
            else:
                data = func(arg)
                try:
                    if data:
                        memcache.add(key, data, expires, namespace=namespace)
                    else:
                        memcache.add(key, data, neg_expires, namespace=namespace)
                except ValueError:
                    logging.exception('unable to write to memcache')
                return data
        return wrapped
    return wrapper


@memcache_memoize('gs-ls://', expires=60)
def gcs_ls(path):
    """Enumerate files in a GCS directory. Returns a list of FileStats."""
    if path[-1] != '/':
        path += '/'
    return list(gcs.listbucket(path, delimiter='/'))


def parse_junit(xml, filename):
    """Generate failed tests as a series of (name, duration, text, filename) tuples."""
    tree = ET.fromstring(xml)
    if tree.tag == 'testsuite':
        for child in tree:
            name = child.attrib['name']
            time = float(child.attrib['time'])
            for param in child.findall('failure'):
                yield name, time, param.text, filename
    elif tree.tag == 'testsuites':
        for testsuite in tree:
            suite_name = testsuite.attrib['name']
            for child in testsuite.findall('testcase'):
                name = '%s %s' % (suite_name, child.attrib['name'])
                time = float(child.attrib['time'])
                for param in child.findall('failure'):
                    yield name, time, param.text, filename
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
        build_log: a hilighted portion of errors in the build log. May be None.
    """
    started_fut = gcs_async.read(build_dir + '/started.json')
    finished = gcs_async.read(build_dir + '/finished.json').get_result()
    started = started_fut.get_result()
    if finished and not started:
        started = 'null'
    if started and not finished:
        finished = 'null'
    elif not (started and finished):
        return
    started = json.loads(started)
    finished = json.loads(finished)

    failures = []
    junit_paths = [f.filename for f in gcs_ls('%s/artifacts' % build_dir)
                   if re.match(r'junit_.*\.xml', os.path.basename(f.filename))]

    junit_futures = {}
    for f in junit_paths:
        junit_futures[gcs_async.read(f)] = f

    for future in junit_futures:
        junit = future.get_result()
        if junit is None:
            continue
        failures.extend(parse_junit(junit, junit_futures[future]))
    failures.sort()

    build_log = None
    if finished and finished.get('result') != 'SUCCESS' and len(failures) == 0:
        build_log = gcs_async.read(build_dir + '/build-log.txt').get_result()
        if build_log:
            build_log = log_parser.digest(build_log.decode('utf8', 'replace'))
            logging.info('fallback log parser emitted %d lines',
                         build_log.count('\n'))
    return started, finished, failures, build_log


@memcache_memoize('log-file-junit://', expires=60*60*4)
def find_log_junit((build_dir, junit, log_file)):
    '''
    Looks in build_dir for log_file in a folder that
    also includes the junit file.
    '''
    tmps = [f.filename for f in gcs_ls('%s/artifacts' % build_dir)
            if '/tmp-node' in f.filename]
    for folder in tmps:
        filenames = [f.filename for f in gcs_ls(folder)]
        if folder + junit in filenames:
            path = folder + log_file
            if path in filenames:
                return path


def find_log_files(all_logs, log_file):
    '''
    Returns list of files named log_file from values in all_logs
    '''
    log_files = []
    for folder in all_logs.itervalues():
        for log in folder:
            if log_file in log:
                log_files.append(log)

    return log_files


@memcache_memoize('all-logs://', expires=60*60*4)
def get_all_logs((directory, artifacts)):
    '''
    returns dictionary given the artifacts folder with the keys being the
    folders, and the values being the log files within the corresponding folder
    '''
    log_files = {}
    if artifacts:
        dirs = [f.filename for f in gcs_ls('%s/artifacts' % directory)
                if f.is_dir]
    else:
        dirs = [directory]
    for d in dirs:
        log_files[d] = []
        for f in gcs_ls(d):
            log_name = regex.log_re.search(f.filename)
            if log_name:
                log_files[d].append(f.filename)
    return log_files


def parse_log_file(log_filename, pod, filters=None, make_dict=False, objref_dict=None):
    """Based on make_dict, either returns the objref_dict or the parsed log file"""
    log = gcs_async.read(log_filename).get_result()
    if log is None:
        return {}, False if make_dict else None
    pod_re = regex.wordRE(pod)
    if objref_dict is None:
        objref_dict = {}
    if make_dict:
        return kubelet_parser.make_dict(log.decode('utf8', 'replace'), pod_re, objref_dict)
    else:
        return log_parser.digest(log.decode('utf8', 'replace'),
        error_re=pod_re, filters=filters, objref_dict=objref_dict)


def get_logs_junit((log_files, pod_name, filters, objref_dict, apiserver_filename)):
    '''
    Get the logs in the case where the junit file with the failure is in a specific folder
    '''
    all_logs = {}
    results = {}

    # default to filtering kube-apiserver log if user unchecks both checkboxes
    if log_files == []:
        log_files = [apiserver_filename]

    artifact_filename = os.path.dirname(apiserver_filename)
    all_logs = get_all_logs((artifact_filename, False))
    parsed_dict, _ = parse_log_file(os.path.join(artifact_filename, "kubelet.log"),
        pod_name, make_dict=True)
    objref_dict.update(parsed_dict)
    if log_files:
        for log_file in log_files:
            parsed_file = parse_log_file(log_file, pod_name, filters, objref_dict=objref_dict)
            if parsed_file:
                results[log_file] = parsed_file

    return all_logs, results, objref_dict, log_files


def get_logs(build_dir, log_files, pod_name, filters, objref_dict):
    '''
    Get the logs in the case where all logs in artifacts folder may be relevant
    '''
    all_logs = {}
    results = {}
    old_dict_len = len(objref_dict)

    all_logs = get_all_logs((build_dir, True))
    apiserver_filename = find_log_files(all_logs, "kube-apiserver.log")
    kubelet_filenames = find_log_files(all_logs, "kubelet.log")
    for kubelet_log in kubelet_filenames:
        parsed_dict, pod_in_file = parse_log_file(kubelet_log, pod_name, make_dict=True)
        objref_dict.update(parsed_dict)
        if len(objref_dict) > old_dict_len or pod_in_file:
            if log_files == []:
                log_files = [kubelet_log]
                if apiserver_filename:
                    log_files.extend(apiserver_filename)
            for log_file in log_files:
                parsed_file = parse_log_file(log_file, pod_name, filters,
                    objref_dict=objref_dict)
                if parsed_file:
                    results[log_file] = parsed_file
            break

    return all_logs, results, objref_dict, log_files


@memcache_memoize('pr-details://', expires=60 * 3)
def pr_builds(pr):
    """
    Get information for all builds run by a PR.

    Args:
        pr: the PR number
    Returns:
        A dictionary of {job: [(build_number, started_json, finished.json)]}
    """
    jobs_dirs_fut = gcs_async.listdirs('%s/%s' % (PR_PREFIX, pr))

    def base(path):
        return os.path.basename(os.path.dirname(path))

    jobs_futures = [(job, gcs_async.listdirs(job)) for job in jobs_dirs_fut.get_result()]
    futures = []

    for job, builds_fut in jobs_futures:
        for build in builds_fut.get_result():
            sta_fut = gcs_async.read('/%sstarted.json' % build)
            fin_fut = gcs_async.read('/%sfinished.json' % build)
            futures.append([base(job), base(build), sta_fut, fin_fut])

    futures.sort(key=lambda (job, build, s, f): (job, pad_numbers(build)), reverse=True)

    jobs = {}
    for job, build, started_fut, finished_fut in futures:
        started = started_fut.get_result()
        finished = finished_fut.get_result()
        if started is not None:
            started = json.loads(started)
        if finished is not None:
            finished = json.loads(finished)
        jobs.setdefault(job, []).append((build, started, finished))

    return jobs


class RenderingHandler(webapp2.RequestHandler):
    """Base class for Handlers that render Jinja templates."""
    def __init__(self, *args, **kwargs):
        super(RenderingHandler, self).__init__(*args, **kwargs)
        # The default deadline of 5 seconds is too aggressive of a target for GCS
        # directory listing operations.
        urlfetch.set_default_fetch_deadline(60)

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
            logging.warning('unable to load %s', build_dir)
            self.render('build_404.html', {"build_dir": build_dir})
            self.response.set_status(404)
            return
        started, finished, failures, build_log = details

        if started:
            commit = started['version'].split('+')[-1]
        else:
            commit = None

        pr = None
        if prefix.startswith(PR_PREFIX):
            pr = os.path.basename(prefix)
        self.render('build.html', dict(
            job_dir=job_dir, build_dir=build_dir, job=job, build=build,
            commit=commit, started=started, finished=finished,
            failures=failures, build_log=build_log, pr=pr))


class BuildListHandler(RenderingHandler):
    """Show a list of Builds for a Job."""
    def get(self, prefix, job):
        self.check_bucket(prefix)
        job_dir = '/%s/%s/' % (prefix, job)
        fstats = gcs_ls(job_dir)
        fstats.sort(key=lambda f: pad_numbers(f.filename), reverse=True)
        self.render('build_list.html',
                    dict(job=job, job_dir=job_dir, fstats=fstats))


class NodeLogHandler(RenderingHandler):
    def get(self, prefix, job, build):
        """
        Examples of variables
        log_files: ["kubelet.log", "kube-apiserver.log"]
        pod_name: "pod-abcdef123"
        junit: "junit_01.xml"
        uid, namespace, wrap: "on"
        logs: {"kubelet.log":"parsed kubelet log for html"}
        all_logs: {"folder_name":["a.log", "b.log"]}
        """
        # pylint: disable=too-many-locals
        self.check_bucket(prefix)
        job_dir = '/%s/%s/' % (prefix, job)
        build_dir = job_dir + build
        log_files = self.request.get_all("logfiles")
        pod_name = self.request.get("pod")
        junit = self.request.get("junit")
        cID = self.request.get("cID")
        poduid = self.request.get("poduid")
        ns = self.request.get("ns")
        uid = bool(self.request.get("UID"))
        namespace = bool(self.request.get("Namespace"))
        containerID = bool(self.request.get("ContainerID"))
        wrap = bool(self.request.get("wrap"))
        filters = {"UID":uid, "pod":pod_name, "Namespace":namespace, "ContainerID":containerID}
        objref_dict = {}

        if cID:
            objref_dict["ContainerID"] = cID
        if poduid:
            objref_dict["UID"] = poduid
        if ns:
            objref_dict["Namespace"] = ns

        apiserver_filename = find_log_junit((build_dir, junit, "kube-apiserver.log"))
        if apiserver_filename and pod_name:
            all_logs, results, objref_dict, log_files = get_logs_junit((log_files,
                pod_name, filters, objref_dict, apiserver_filename))
        if not apiserver_filename:
            all_logs, results, objref_dict, log_files = get_logs(build_dir, log_files,
                pod_name, filters, objref_dict)

        if results == {}:
            self.render('node_404.html', {"build_dir": build_dir, "log_files": log_files,
                "pod_name":pod_name, "junit":junit})
            self.response.set_status(404)
            return

        self.render('filtered_log.html', dict(
            job_dir=job_dir, build_dir=build_dir, logs=results, job=job,
            build=build, log_files=log_files, containerID=containerID,
            pod=pod_name, junit=junit, uid=uid, namespace=namespace,
            wrap=wrap, objref_dict=objref_dict, all_logs=all_logs))


class JobListHandler(RenderingHandler):
    """Show a list of Jobs in a directory."""
    def get(self, prefix):
        self.check_bucket(prefix)
        jobs_dir = '/%s' % prefix
        fstats = gcs_ls(jobs_dir)
        fstats.sort()
        self.render('job_list.html', dict(jobs_dir=jobs_dir, fstats=fstats))


class PRHandler(RenderingHandler):
    """Show a list of test runs for a PR."""
    def get(self, pr):
        builds = pr_builds(pr)
        max_builds, headings, rows = pull_request.builds_to_table(builds)
        self.render('pr.html', dict(pr=pr, prefix=PR_PREFIX,
            max_builds=max_builds, header=headings, rows=rows))

class PRDashboard(RenderingHandler):
    def get(self, user=None):
        # pylint: disable=singleton-comparison
        qs = [ghm.GHIssueDigest.is_pr == True]
        if not self.request.get('all', False):
            qs.append(ghm.GHIssueDigest.is_open == True)
        if user is not None:
            qs.append(ghm.GHIssueDigest.involved == user)
        prs = list(ghm.GHIssueDigest.query(*qs))
        prs.sort(key=lambda x: x.updated_at, reverse=True)
        trim = 0
        if all(pr.repo.startswith('kubernetes/') for pr in prs):
            trim = len('kubernetes/')
        if user:
            cats = [
                ('Needs Attention', lambda p: user in p.payload['attn'], ''),
                ('Incoming', lambda p: user in p.payload['assignees'],
                 'is:open is:pr user:kubernetes assignee:%s' % user),
                ('Outgoing', lambda p: user == p.payload['author'],
                 'is:open is:pr user:kubernetes author:%s' % user),
            ]
        else:
            cats = [('Open Kubernetes PRs', lambda x: True,
                'is:open is:pr user:kubernetes')]

        self.render('pr_dashboard.html', dict(prs=prs,
            cats=cats, trim=trim, user=user))

class PRBuildLogHandler(webapp2.RequestHandler):
    def get(self, path):
        self.redirect('https://storage.googleapis.com/%s/%s' % (PR_PREFIX, path))


app = webapp2.WSGIApplication([
    (r'/', IndexHandler),
    (r'/jobs/(.*)$', JobListHandler),
    (r'/builds/(.*)/([^/]+)/?', BuildListHandler),
    (r'/build/(.*)/([^/]+)/(\d+)/?', BuildHandler),
    (r'/build/(.*)/([^/]+)/(\d+)/nodelog*', NodeLogHandler),
    (r'/pr/(\d+)', PRHandler),
    (r'/pr/?', PRDashboard),
    (r'/pr/([-\w]+)', PRDashboard),
    (r'/pr/(.*/build-log.txt)', PRBuildLogHandler),
], debug=True)
