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

import os
import unittest

import cloudstorage as gcs

import view_build

import main_test
import gcs_async_test
import github.models
import testgrid_test

app = main_test.app
init_build = main_test.init_build
write = gcs_async_test.write

class ParseJunitTest(unittest.TestCase):
    @staticmethod
    def parse(xml):
        parser = view_build.JUnitParser()
        parser.parse_xml(xml, 'junit_filename.xml')
        return parser.get_results()

    def test_normal(self):
        results = self.parse(main_test.JUNIT_SUITE)
        stack = '/go/src/k8s.io/kubernetes/test.go:123\nError Goes Here'
        self.assertEqual(results, {
            'passed': ['Second'],
            'skipped': ['First'],
            'failed': [('Third', 96.49, stack, "junit_filename.xml", "")],
        })

    def test_testsuites(self):
        results = self.parse("""
            <testsuites>
                <testsuite name="k8s.io/suite">
                    <properties>
                        <property name="go.version" value="go1.6"/>
                    </properties>
                    <testcase name="TestBad" time="0.1">
                        <failure>something bad</failure>
                        <system-out>out: first line</system-out>
                        <system-err>err: first line</system-err>
                        <system-out>out: second line</system-out>
                    </testcase>
                </testsuite>
            </testsuites>""")
        self.assertEqual(results['failed'], [(
            'k8s.io/suite TestBad', 0.1, 'something bad', "junit_filename.xml",
            "out: first line\nout: second line\nerr: first line",
            )])

    def test_testsuites_no_time(self):
        results = self.parse("""
            <testsuites>
                <testsuite name="k8s.io/suite">
                    <properties>
                        <property name="go.version" value="go1.6"/>
                    </properties>
                    <testcase name="TestBad">
                        <failure>something bad</failure>
                        <system-out>out: first line</system-out>
                        <system-err>err: first line</system-err>
                        <system-out>out: second line</system-out>
                    </testcase>
                </testsuite>
            </testsuites>""")
        self.assertEqual(results['failed'], [(
            'k8s.io/suite TestBad', 0.0, 'something bad', "junit_filename.xml",
            "out: first line\nout: second line\nerr: first line",
            )])


    def test_nested_testsuites(self):
        results = self.parse("""
            <testsuites>
                <testsuite name="k8s.io/suite">
                    <testsuite name="k8s.io/suite/sub">
                        <properties>
                            <property name="go.version" value="go1.6"/>
                        </properties>
                        <testcase name="TestBad" time="0.1">
                            <failure>something bad</failure>
                            <system-out>out: first line</system-out>
                            <system-err>err: first line</system-err>
                            <system-out>out: second line</system-out>
                        </testcase>
                    </testsuite>
                </testsuite>
            </testsuites>""")
        self.assertEqual(results['failed'], [(
            'k8s.io/suite/sub TestBad', 0.1, 'something bad', "junit_filename.xml",
            "out: first line\nout: second line\nerr: first line",
            )])

    def test_bad_xml(self):
        self.assertEqual(self.parse("""<body />""")['failed'], [])

    def test_corrupt_xml(self):
        self.assertEqual(self.parse('<a>\xff</a>')['failed'], [])
        failures = self.parse("""
            <testsuites>
                <testsuite name="a">
                    <testcase name="Corrupt" time="0">
                        <failure>something bad \xff</failure>
                    </testcase>
                </testsuite>
            </testsuites>""")['failed']
        self.assertEqual(failures, [('a Corrupt', 0.0, 'something bad ?',
                                     'junit_filename.xml', '')])

    def test_not_xml(self):
        failures = self.parse('\x01')['failed']
        self.assertEqual(failures,
            [(failures[0][0], 0.0, 'not well-formed (invalid token): line 1, column 0',
              'junit_filename.xml', '')])

    def test_empty_output(self):
        results = self.parse("""
            <testsuites>
                <testsuite name="k8s.io/suite">
                    <testcase name="TestBad" time="0.1">
                        <failure>something bad</failure>
                        <system-out></system-out>
                    </testcase>
                </testsuite>
            </testsuites>""")
        self.assertEqual(results['failed'], [(
            'k8s.io/suite TestBad', 0.1, 'something bad', "junit_filename.xml", "")])

class BuildTest(main_test.TestBase):
    # pylint: disable=too-many-public-methods

    JOB_DIR = '/kubernetes-jenkins/logs/somejob/'
    BUILD_DIR = JOB_DIR + '1234/'

    def setUp(self):
        self.init_stubs()
        init_build(self.BUILD_DIR)
        testgrid_test.write_config()

    def get_build_page(self, trailing=''):
        return app.get('/build' + self.BUILD_DIR + trailing)

    def test_missing(self):
        """Test that a missing build gives a 404."""
        response = app.get('/build' + self.BUILD_DIR.replace('1234', '1235'),
                           status=404)
        self.assertIn('1235', response)

    def test_missing_started(self):
        """Test that a missing started.json still renders a proper page."""
        build_dir = '/kubernetes-jenkins/logs/job-with-no-started/1234/'
        init_build(build_dir, started=False)
        response = app.get('/build' + build_dir)
        self.assertRegexpMatches(response.body, 'Result.*SUCCESS')
        self.assertIn('job-with-no-started', response)
        self.assertNotIn('Started', response)  # no start timestamp
        self.assertNotIn('github.com', response)  # no version => no src links

    def test_missing_finished(self):
        """Test that a missing finished.json still renders a proper page."""
        build_dir = '/kubernetes-jenkins/logs/job-still-running/1234/'
        init_build(build_dir, finished=False)
        response = app.get('/build' + build_dir)
        self.assertRegexpMatches(response.body, 'Result.*Not Finished')
        self.assertIn('job-still-running', response)
        self.assertIn('Started', response)

    def test_build(self):
        """Test that the build page works in the happy case."""
        response = self.get_build_page()
        self.assertIn('2014-07-28', response)  # started
        self.assertIn('v1+56', response)       # build version
        self.assertIn('16m40s', response)      # build duration
        self.assertIn('Third', response)       # test name
        self.assertIn('1m36s', response)       # test duration
        self.assertRegexpMatches(response.body, 'Result.*SUCCESS')
        self.assertIn('Error Goes Here', response)
        self.assertIn('test.go#L123">', response)  # stacktrace link works

    def test_finished_has_version(self):
        """Test that metadata with version in finished works."""
        init_build(self.BUILD_DIR, finished_has_version=True)
        self.test_build()

    def test_build_no_failures(self):
        """Test that builds with no Junit artifacts work."""
        gcs.delete(self.BUILD_DIR + 'artifacts/junit_01.xml')
        response = self.get_build_page()
        self.assertIn('No Test Failures', response)

    def test_show_metadata(self):
        write(self.BUILD_DIR + 'started.json',
            {
                'version': 'v1+56',
                'timestamp': 1406535800,
                'node': 'agent-light-7',
                'pull': 'master:1234,35:abcd,72814',
                'metadata': {
                    'master-version': 'm12'
                }
            })
        write(self.BUILD_DIR + 'finished.json',
            {
                'timestamp': 1406536800,
                'passed': True,
                'metadata': {
                    'skew-version': 'm11'
                }
            })
        response = self.get_build_page()
        self.assertIn('v1+56', response)
        self.assertIn('agent-light-7', response)
        self.assertIn('<td>master-version<td>m12', response)
        self.assertIn('<td>skew-version<td>m11', response)
        self.assertIn('1234', response)
        self.assertIn('abcd', response)
        self.assertIn('72814', response)

    def test_build_show_log(self):
        """Test that builds that failed with no failures show the build log."""
        gcs.delete(self.BUILD_DIR + 'artifacts/junit_01.xml')
        write(self.BUILD_DIR + 'finished.json',
              {'passed': False, 'timestamp': 1406536800})

        # Unable to fetch build-log.txt, still works.
        response = self.get_build_page()
        self.assertNotIn('Error lines', response)

        self.testbed.init_memcache_stub()  # clear cached result
        write(self.BUILD_DIR + 'build-log.txt',
              u'ERROR: test \u039A\n\n\n\n\n\n\n\n\nblah'.encode('utf8'))
        response = self.get_build_page()
        self.assertIn('Error lines', response)
        self.assertIn('No Test Failures', response)
        self.assertIn('ERROR</span>: test', response)
        self.assertNotIn('blah', response)

    def test_build_show_passed_skipped(self):
        response = self.get_build_page()
        self.assertIn('First', response)
        self.assertIn('Second', response)
        self.assertIn('Third', response)
        self.assertIn('Show 1 Skipped Tests', response)
        self.assertIn('Show 1 Passed Tests', response)

    def test_build_optional_log(self):
        """Test that passing builds do not show logs by default but display them when requested"""
        write(self.BUILD_DIR + 'build-log.txt', 'error or timeout or something')
        response = self.get_build_page()
        self.assertIn('<a href="?log#log">', response)
        self.assertNotIn('timeout', response)
        self.assertNotIn('build-log.txt', response)
        response = self.get_build_page('?log')
        self.assertIn('timeout', response)
        self.assertIn('build-log.txt', response)

    def test_build_testgrid_links(self):
        response = self.get_build_page()
        base = 'https://testgrid.k8s.io/k8s#ajob'
        self.assertIn('a href="%s"' % base, response)
        option = '&amp;include-filter-by-regex=%5EOverall%24%7CThird'
        self.assertIn('a href="%s%s"' % (base, option), response)

    def test_build_failure_no_text(self):
        # Some failures don't have any associated text.
        write(self.BUILD_DIR + 'artifacts/junit_01.xml', """
            <testsuites>
                <testsuite tests="1" failures="1" time="3.274" name="k8s.io/test/integration">
                    <testcase classname="integration" name="TestUnschedulableNodes" time="0.210">
                        <failure message="Failed" type=""/>
                    </testcase>
                </testsuite>
            </testsuites>""")
        response = self.get_build_page()
        self.assertIn('TestUnschedulableNodes', response)
        self.assertIn('junit_01.xml', response)

    def test_build_empty_junit(self):
        # Sometimes junit files are actually empty (???)
        write(self.BUILD_DIR + 'artifacts/junit_01.xml', '')
        response = self.get_build_page()
        print response
        self.assertIn('No Test Failures', response)

    def test_parse_pr_path(self):
        def check(prefix, expected):
            self.assertEqual(
                view_build.parse_pr_path(gcs_path=prefix,
                    default_org='kubernetes',
                    default_repo='kubernetes',
                ),
                expected
            )

        check('kubernetes-jenkins/pr-logs/pull/123', ('123', '', 'kubernetes/kubernetes'))
        check('kubernetes-jenkins/pr-logs/pull/charts/123', ('123', 'charts/', 'kubernetes/charts'))
        check(
            'kubernetes-jenkins/pr-logs/pull/google_cadvisor/296',
            ('296', 'google/cadvisor/', 'google/cadvisor'))
        check(
            'kj/pr-logs/pull/kubernetes-sigs_testing_frameworks/49',
            ('49', 'kubernetes-sigs/testing_frameworks/', 'kubernetes-sigs/testing_frameworks'))

    def test_github_commit_links(self):
        def check(build_dir, result):
            init_build(build_dir)
            response = app.get('/build' + build_dir)
            self.assertIn(result, response)

        check('/kubernetes-jenkins/logs/ci-kubernetes-e2e/2/',
               'github.com/kubernetes/kubernetes/commit/')
        check('/kubernetes-jenkins/pr-logs/pull/charts/123/e2e/40/',
               'github.com/kubernetes/charts/commit/')
        check('/kubernetes-jenkins/pr-logs/pull/google_cadvisor/432/e2e/296/',
               'github.com/google/cadvisor/commit/')

    def test_build_pr_link(self):
        """ The build page for a PR build links to the PR results."""
        build_dir = '/kubernetes-jenkins/pr-logs/pull/123/e2e/567/'
        init_build(build_dir)
        response = app.get('/build' + build_dir)
        self.assertIn('PR #123', response)
        self.assertIn('href="/pr/123"', response)

    def test_build_pr_link_other(self):
        build_dir = '/kubernetes-jenkins/pr-logs/pull/charts/123/e2e/567/'
        init_build(build_dir)
        response = app.get('/build' + build_dir)
        self.assertIn('PR #123', response)
        self.assertIn('href="/pr/charts/123"', response)

    def test_build_xref(self):
        """Test that builds show issues that reference them."""
        github.models.GHIssueDigest.make(
            'org/repo', 123, True, True, [],
            {'xrefs': [self.BUILD_DIR[:-1]], 'title': 'an update on testing'}, None).put()
        response = app.get('/build' + self.BUILD_DIR)
        self.assertIn('PR #123', response)
        self.assertIn('an update on testing', response)
        self.assertIn('org/repo/issues/123', response)

    def test_build_list_xref(self):
        """Test that builds show issues that reference them."""
        github.models.GHIssueDigest.make(
            'org/repo', 123, False, True, [],
            {'xrefs': [self.BUILD_DIR[:-1]], 'title': 'an update on testing'}, None).put()
        response = app.get('/builds' + self.JOB_DIR)
        self.assertIn('#123', response)
        self.assertIn('an update on testing', response)
        self.assertIn('org/repo/issues/123', response)

    def test_cache(self):
        """Test that caching works at some level."""
        response = self.get_build_page()
        gcs.delete(self.BUILD_DIR + 'started.json')
        gcs.delete(self.BUILD_DIR + 'finished.json')
        response2 = self.get_build_page()
        self.assertEqual(str(response), str(response2))

    def test_build_directory_redir(self):
        build_dir = '/kubernetes-jenkins/pr-logs/directory/somejob/1234'
        target_dir = '/kubernetes-jenkins/pr-logs/pull/45/somejob/1234'
        write(build_dir + '.txt', 'gs:/' + target_dir)
        resp = app.get('/build' + build_dir)
        self.assertEqual(resp.status_code, 302)
        self.assertEqual(resp.location, 'http://localhost/build' + target_dir)

    def do_view_build_list_test(self, job_dir='/buck/some-job/', indirect=False):
        sta_result = {'timestamp': 12345}
        fin_result = {'passed': True, 'result': 'SUCCESS'}
        for n in xrange(120):
            write('%s%d/started.json' % (job_dir, n), sta_result)
            write('%s%d/finished.json' % (job_dir, n), fin_result)
        if indirect:
            for n in xrange(120):
                write('%sdirectory/%d.txt' % (job_dir, n), 'gs:/%s%d' % (job_dir, n))

        view_target = job_dir if not indirect else job_dir + 'directory/'

        builds, _ = view_build.build_list(view_target, None)
        self.assertEqual(builds,
                         [(str(n), '%s%s' % (job_dir, n), sta_result, fin_result)
                          for n in range(119, 79, -1)])
        # test that ?before works
        builds, _ = view_build.build_list(view_target, '80')
        self.assertEqual(builds,
                         [(str(n), '%s%s' % (job_dir, n), sta_result, fin_result)
                          for n in range(79, 39, -1)])

    def test_view_build_list_with_latest(self):
        write('/buck/some-job/latest-build.txt', '119')
        self.do_view_build_list_test()

    def test_view_build_list_with_old_latest(self):
        # latest-build.txt is a hint -- it will probe newer by looking for started.json
        write('/buck/some-job/latest-build.txt', '110')
        self.do_view_build_list_test()

    def test_view_build_list_no_latest(self):
        self.do_view_build_list_test()

    def test_view_build_list_indirect_with_latest(self):
        write('/buck/some-job/directory/latest-build.txt', '119')
        self.do_view_build_list_test(indirect=True)

    def test_view_build_list_indirect_no_latest(self):
        self.do_view_build_list_test(indirect=True)

    def test_build_list_handler(self):
        """Test that the job page shows a list of builds."""
        response = app.get('/builds' + os.path.dirname(self.BUILD_DIR[:-1]))
        self.assertIn('/1234/">1234', response)
        self.assertIn('gcsweb', response)

    def test_job_list(self):
        """Test that the job list shows our job."""
        response = app.get('/jobs/kubernetes-jenkins/logs')
        self.assertIn('somejob/">somejob</a>', response)

    def test_recent_runs_across_prs(self):
        """Test that "Recent Runs Across PRs" links are correct."""
        def expect(path, directory):
            response = app.get('/builds/' + path)
            self.assertIn('href="/builds/%s"' % directory, response)
        # pull request job in main repo
        expect(
            'k-j/pr-logs/pull/514/pull-kubernetes-unit/',
            'k-j/pr-logs/directory/pull-kubernetes-unit')
        # pull request jobs in different repos
        expect(
            'k-j/pr-logs/pull/test-infra/4213/pull-test-infra-bazel',
            'k-j/pr-logs/directory/pull-test-infra-bazel')
        expect(
            'i-p/pull/istio_istio/517/istio-presubmit/',
            'i-p/directory/istio-presubmit')
