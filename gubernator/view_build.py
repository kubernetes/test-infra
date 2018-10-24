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
import json
import os
import re

import defusedxml.ElementTree as ET

from google.appengine.api import urlfetch

import gcs_async
from github import models
import log_parser
import testgrid
import view_base


class JUnitParser(object):
    def __init__(self):
        self.skipped = []
        self.passed = []
        self.failed = []

    def handle_suite(self, tree, filename):
        for subelement in tree:
            if subelement.tag == 'testsuite':
                self.handle_suite(subelement, filename)
            elif subelement.tag == 'testcase':
                if 'name' in tree.attrib:
                    name_prefix = tree.attrib['name'] + ' '
                else:
                    name_prefix = ''
                self.handle_test(subelement, filename, name_prefix)

    def handle_test(self, child, filename, name_prefix=''):
        name = name_prefix + child.attrib['name']
        if child.find('skipped') is not None:
            self.skipped.append(name)
        elif child.find('failure') is not None:
            time = 0.0
            if 'time' in child.attrib:
                time = float(child.attrib['time'])
            out = []
            for param in child.findall('system-out') + child.findall('system-err'):
                if param.text:
                    out.append(param.text)
            for param in child.findall('failure'):
                self.failed.append((name, time, param.text, filename, '\n'.join(out)))
        else:
            self.passed.append(name)

    def parse_xml(self, xml, filename):
        if not xml:
            return  # can't extract results from nothing!
        try:
            tree = ET.fromstring(xml)
        except ET.ParseError, e:
            logging.exception('parse_junit failed for %s', filename)
            try:
                tree = ET.fromstring(re.sub(r'[\x00\x80-\xFF]+', '?', xml))
            except ET.ParseError, e:
                if re.match(r'junit.*\.xml', os.path.basename(filename)):
                    self.failed.append(
                        ('Gubernator Internal Fatal XML Parse Error', 0.0, str(e), filename, ''))
                return
        if tree.tag == 'testsuite':
            self.handle_suite(tree, filename)
        elif tree.tag == 'testsuites':
            for testsuite in tree:
                self.handle_suite(testsuite, filename)
        else:
            logging.error('unable to find failures, unexpected tag %s', tree.tag)

    def get_results(self):
        self.failed.sort()
        self.skipped.sort()
        self.passed.sort()
        return {
            'failed': self.failed,
            'skipped': self.skipped,
            'passed': self.passed,
        }


@view_base.memcache_memoize('build-log-parsed://', expires=60*60*4)
def get_build_log(build_dir):
    build_log = gcs_async.read(build_dir + '/build-log.txt').get_result()
    if build_log:
        return log_parser.digest(build_log)


def get_running_build_log(job, build, prow_url):
    try:
        url = "https://%s/log?job=%s&id=%s" % (prow_url, job, build)
        result = urlfetch.fetch(url)
        if result.status_code == 200:
            return log_parser.digest(result.content), url
    except urlfetch.Error:
        logging.exception('Caught exception fetching url')
    return None, None


def normalize_metadata(started_future, finished_future):
    """
    Munge and normalize the output of loading started
    and finished.json files from a GCS bucket.

    :param started_future: future from gcs_async.read()
    :param finished_future: future from gcs_async.read()
    :return: started, finished dictionaries
    """
    started = started_future.get_result()
    finished = finished_future.get_result()
    if finished and not started:
        started = 'null'
    elif started and not finished:
        finished = 'null'
    elif not (started and finished):
        return None, None
    started = json.loads(started)
    finished = json.loads(finished)

    if finished is not None:
        # we want to allow users pushing to GCS to
        # provide us either passed or result, but not
        # require either (or both)
        if 'result' in finished and 'passed' not in finished:
            finished['passed'] = finished['result'] == 'SUCCESS'

        if 'passed' in finished and 'result' not in finished:
            finished['result'] = 'SUCCESS' if finished['passed'] else 'FAILURE'

    return started, finished


@view_base.memcache_memoize('build-details://', expires=60)
def build_details(build_dir, recursive=False):
    """
    Collect information from a build directory.

    Args:
        build_dir: GCS path containing a build's results.
        recursive: Whether to scan artifacts recursively for XML files.
    Returns:
        started: value from started.json {'version': ..., 'timestamp': ...}
        finished: value from finished.json {'timestamp': ..., 'result': ...}
        results: {total: int,
                  failed: [(name, duration, text)...],
                  skipped: [name...],
                  passed: [name...]}
    """
    started, finished = normalize_metadata(
        gcs_async.read(build_dir + '/started.json'),
        gcs_async.read(build_dir + '/finished.json')
    )

    if started is None and finished is None:
        return started, finished, None

    if recursive:
        artifact_paths = view_base.gcs_ls_recursive('%s/artifacts' % build_dir)
    else:
        artifact_paths = view_base.gcs_ls('%s/artifacts' % build_dir)

    junit_paths = [f.filename for f in artifact_paths if f.filename.endswith('.xml')]

    junit_futures = {f: gcs_async.read(f) for f in junit_paths}

    parser = JUnitParser()
    for path, future in junit_futures.iteritems():
        parser.parse_xml(future.get_result(), path)
    return started, finished, parser.get_results()


def parse_pr_path(gcs_path, default_org, default_repo):
    """
    Parse GCS bucket directory into metadata. We
    allow for two short-form names and one long one:

     gs://<pull_prefix>/<pull_number>
      -- this fills in the default repo and org

     gs://<pull_prefix>/repo/<pull_number>
      -- this fills in the default org

     gs://<pull_prefix>/org_repo/<pull_number>

    :param gcs_path: GCS bucket directory for a build
    :return: tuple of:
     - PR number
     - Gubernator PR link
     - PR repo
    """
    pull_number = os.path.basename(gcs_path)
    parsed_repo = os.path.basename(os.path.dirname(gcs_path))
    if parsed_repo == 'pull':
        pr_path = ''
        repo = '%s/%s' % (default_org, default_repo)
    elif '_' not in parsed_repo:
        pr_path = parsed_repo + '/'
        repo = '%s/%s' % (default_org, parsed_repo)
    else:
        pr_path = parsed_repo.replace('_', '/', 1) + '/'
        repo = parsed_repo.replace('_', '/', 1)
    return pull_number, pr_path, repo


class BuildHandler(view_base.BaseHandler):
    """Show information about a Build and its failing tests."""
    def get(self, prefix, job, build):
        # pylint: disable=too-many-locals
        if prefix.endswith('/directory'):
            # redirect directory requests
            link = gcs_async.read('/%s/%s/%s.txt' % (prefix, job, build)).get_result()
            if link and link.startswith('gs://'):
                self.redirect('/build/' + link.replace('gs://', ''))
                return

        job_dir = '/%s/%s/' % (prefix, job)
        testgrid_query = testgrid.path_to_query(job_dir)
        build_dir = job_dir + build
        issues_fut = models.GHIssueDigest.find_xrefs_async(build_dir)
        started, finished, results = build_details(
            build_dir, self.app.config.get('recursive_artifacts', True))
        if started is None and finished is None:
            logging.warning('unable to load %s', build_dir)
            self.render(
                'build_404.html',
                dict(build_dir=build_dir, job_dir=job_dir, job=job, build=build))
            self.response.set_status(404)
            return

        want_build_log = False
        build_log = ''
        build_log_src = None
        if 'log' in self.request.params or (not finished) or \
            (finished and finished.get('result') != 'SUCCESS' and len(results['failed']) <= 1):
            want_build_log = True
            build_log = get_build_log(build_dir)

        pr, pr_path, pr_digest = None, None, None
        repo = '%s/%s' % (self.app.config['default_org'],
                          self.app.config['default_repo'])
        external_config = get_build_config(prefix, self.app.config)
        if external_config is not None:
            if '/pull/' in prefix:
                pr, pr_path, pr_digest, repo = get_pr_info(prefix, self.app.config)
            if want_build_log and not build_log:
                build_log, build_log_src = get_running_build_log(job, build,
                                                                 external_config["prow_url"])

        # 'version' might be in either started or finished.
        # prefer finished.
        version = finished and finished.get('version') or started and started.get('version')
        commit = version and version.split('+')[-1]

        refs = []
        if started and started.get('pull'):
            for ref in started['pull'].split(','):
                x = ref.split(':', 1)
                if len(x) == 2:
                    refs.append((x[0], x[1]))
                else:
                    refs.append((x[0], ''))

        self.render('build.html', dict(
            job_dir=job_dir, build_dir=build_dir, job=job, build=build,
            commit=commit, started=started, finished=finished,
            res=results, refs=refs,
            build_log=build_log, build_log_src=build_log_src,
            issues=issues_fut.get_result(), repo=repo,
            pr_path=pr_path, pr=pr, pr_digest=pr_digest,
            testgrid_query=testgrid_query))


def get_build_config(prefix, config):
    for item in config['external_services'].values() + [config['default_external_services']]:
        if prefix.startswith(item['gcs_pull_prefix']):
            return item
        if 'gcs_bucket' in item and prefix.startswith(item['gcs_bucket']):
            return item

def get_pr_info(prefix, config):
    if config is not None:
        pr, pr_path, repo = parse_pr_path(
            gcs_path=prefix,
            default_org=config['default_org'],
            default_repo=config['default_repo'],
        )
        pr_digest = models.GHIssueDigest.get(repo, pr)
        return pr, pr_path, pr_digest, repo

def get_running_pr_log(job, build, config):
    if config is not None:
        return get_running_build_log(job, build, config["prow_url"])

def get_build_numbers(job_dir, before, indirect):
    fstats = view_base.gcs_ls(job_dir)
    fstats.sort(key=lambda f: view_base.pad_numbers(f.filename),
                reverse=True)
    if indirect:
        # find numbered builds
        builds = [re.search(r'/(\d*)\.txt$', f.filename)
                  for f in fstats if not f.is_dir]
        builds = [m.group(1) for m in builds if m]
    else:
        builds = [os.path.basename(os.path.dirname(f.filename))
                  for f in fstats if f.is_dir]
    if before and before in builds:
        builds = builds[builds.index(before) + 1:]
    return builds[:40]


@view_base.memcache_memoize('build-list://', expires=60)
def build_list(job_dir, before):
    """
    Given a job dir, give a (partial) list of recent build
    started.json & finished.jsons.

    Args:
        job_dir: the GCS path holding the jobs
    Returns:
        a list of [(build, loc, started, finished)].
            build is a string like "123",
            loc is the job directory and build,
            started/finished are either None or a dict of the finished.json,
        and a dict of {build: [issues...]} of xrefs.
    """
    # pylint: disable=too-many-locals

    # /directory/ folders have a series of .txt files pointing at the correct location,
    # as a sort of fake symlink.
    indirect = '/directory/' in job_dir

    builds = get_build_numbers(job_dir, before, indirect)

    if indirect:
        # follow the indirect links
        build_symlinks = [
            (build,
             gcs_async.read('%s%s.txt' % (job_dir, build)))
            for build in builds
        ]
        build_futures = []
        for build, sym_fut in build_symlinks:
            redir = sym_fut.get_result()
            if redir and redir.startswith('gs://'):
                redir = redir[4:].strip()
                build_futures.append(
                    (build, redir,
                     gcs_async.read('%s/started.json' % redir),
                     gcs_async.read('%s/finished.json' % redir)))
    else:
        build_futures = [
            (build, '%s%s' % (job_dir, build),
             gcs_async.read('%s%s/started.json' % (job_dir, build)),
             gcs_async.read('%s%s/finished.json' % (job_dir, build)))
            for build in builds
        ]

    # This is done in parallel with waiting for GCS started/finished.
    build_refs = models.GHIssueDigest.find_xrefs_multi_async(
            [b[1] for b in build_futures])

    output = []
    for build, loc, started_future, finished_future in build_futures:
        started, finished = normalize_metadata(started_future, finished_future)
        output.append((str(build), loc, started, finished))

    return output, build_refs.get_result()

class BuildListHandler(view_base.BaseHandler):
    """Show a list of Builds for a Job."""
    def get(self, prefix, job):
        job_dir = '/%s/%s/' % (prefix, job)
        testgrid_query = testgrid.path_to_query(job_dir)
        before = self.request.get('before')
        builds, refs = build_list(job_dir, before)
        dir_link = re.sub(r'/pull/.*', '/directory/%s' % job, prefix)

        self.render('build_list.html',
                    dict(job=job, job_dir=job_dir, dir_link=dir_link,
                         testgrid_query=testgrid_query,
                         builds=builds, refs=refs,
                         before=before))


class JobListHandler(view_base.BaseHandler):
    """Show a list of Jobs in a directory."""
    def get(self, prefix):
        jobs_dir = '/%s' % prefix
        fstats = view_base.gcs_ls(jobs_dir)
        fstats.sort()
        self.render('job_list.html', dict(jobs_dir=jobs_dir, fstats=fstats))


class GcsProxyHandler(view_base.BaseHandler):
    """Proxy results from GCS.

    Useful for buckets that don't have public read permissions."""
    def get(self):
        # let's lock this down to build logs for now.
        path = self.request.get('path')
        if not re.match(r'^[-\w/.]+$', path):
            self.abort(403)
        if not path.endswith('/build-log.txt'):
            self.abort(403)
        content = gcs_async.read(path).get_result()
        # lazy XSS prevention.
        # doesn't work on terrible browsers that do content sniffing (ancient IE).
        self.response.headers['Content-Type'] = 'text/plain'
        self.response.write(content)
