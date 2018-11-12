#!/usr/bin/env python2

# Copyright 2017 The Kubernetes Authors.
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

# pylint: disable=invalid-name,missing-docstring

import json
import os
import unittest
import shutil
import tempfile

import summarize


make_test = lambda t: {'failure_text': t}


class StringsTest(unittest.TestCase):
    def test_normalize(self):
        for src, dst in [
                ('0x1234 a 123.13.45.43 b 2e24e003-9ffd-4e78-852c-9dcb6cbef493-123',
                 'UNIQ1 a UNIQ2 b UNIQ3'),
                ('Mon, 12 January 2017 11:34:35 blah blah', 'TIMEblah blah'),
                ('123.45.68.12:345 abcd1234eeee', 'UNIQ1 UNIQ2'),
                ('foobarbaz ' * 500000,
                 'foobarbaz ' * 500 + '\n...[truncated]...\n' + 'foobarbaz ' * 500),
        ]:
            self.assertEqual(summarize.normalize(src), dst)

    def test_editdist(self):
        for a, b, expected in [
                ('foob', 'food', 1),
                ('doot', 'dot', 1),
                ('foob', 'f', 3),
                ('foob', 'g', 4),
        ]:
            self.assertEqual(summarize.editdist(a, b), expected, (a, b, expected))

    def test_make_ngram_counts(self):
        self.assertEqual(sum(summarize.make_ngram_counts('abcdefg')), 4)
        self.assertEqual(sum(summarize.make_ngram_counts(u'abcdefg')), 4)
        self.assertEqual(sum(summarize.make_ngram_counts(u'abcdefg\u2006')), 5)

    def test_make_ngram_counts_digest(self):
        # ensure stability of ngram count digest
        self.assertEqual(summarize.make_ngram_counts_digest('some string'), 'eddb950347d1eb05b5d7')

    def test_ngram_editdist(self):
        self.assertEqual(summarize.ngram_editdist('example text', 'exampl text'), 1)

    def test_common_spans(self):
        for a, b, expected in [
                ('an exact match', 'an exact match', [14]),
                ('some example string', 'some other string', [5, 7, 7]),
                ('a problem with a common set', 'a common set', [2, 7, 1, 4, 13]),
        ]:
            self.assertEqual(summarize.common_spans([a, b]), expected)


class ClusterTest(unittest.TestCase):
    def test_cluster_test(self):
        # small strings aren't equal, even with tiny differences
        t1 = make_test('exit 1')
        t2 = make_test('exit 2')
        self.assertEqual(summarize.cluster_test([t1, t2]), {'exit 1': [t1], 'exit 2': [t2]})

        t3 = make_test('long message immediately preceding exit code 1')
        t4 = make_test('long message immediately preceding exit code 2')
        self.assertEqual(summarize.cluster_test([t3, t4]), {t3['failure_text']: [t3, t4]})

        t5 = make_test('1 2 ' * 400)
        t6 = make_test('1 2 ' * 399 + '3 4 ')

        self.assertEqual(summarize.cluster_test([t1, t5, t6]),
                         {t1['failure_text']: [t1], t5['failure_text']: [t5, t6]})

    @staticmethod
    def cluster_global(clustered, previous_clustered=None):
        return summarize.cluster_global.__wrapped__(clustered, previous_clustered)

    def test_cluster_global(self):
        t1 = make_test('exit 1')
        t2 = make_test('exit 1')
        t3 = make_test('exit 1')

        self.assertEqual(
            self.cluster_global({'test a': {'exit 1': [t1, t2]}, 'test b': {'exit 1': [t3]}}),
            {'exit 1': {'test a': [t1, t2], 'test b': [t3]}})

    def test_cluster_global_previous(self):
        # clusters are stable when provided with previous seeds
        textOld = 'some long failure message that changes occasionally foo'
        textNew = textOld.replace('foo', 'bar')
        t1 = make_test(textNew)

        self.assertEqual(
            self.cluster_global({'test a': {textNew: [t1]}}, [{'key': textOld}]),
            {textOld: {'test a': [t1]}})

    def test_annotate_owners(self):
        def expect(test, owner, owners=None):
            now = 1.5e9
            data = {
                'builds': {
                    'job_paths': {'somejob': '/logs/somejob'},
                    'cols': {'started': [now]}
                },
                'clustered': [
                    {'tests': [{'name': test, 'jobs': [{'name': 'somejob', 'builds': [123]}]}]}
                ],
            }
            summarize.annotate_owners(
                data, {'/logs/somejob/123': {'started': now}}, owners or {})

            self.assertEqual(owner, data['clustered'][0]['owner'])

        expect('[sig-node] Node reboots', 'node')
        expect('unknown test name', 'testing')
        expect('Suffixes too [sig-storage]', 'storage')
        expect('Variable test with old-style prefixes', 'node', {'node': ['Variable']})


############ decode JSON without a bunch of unicode garbage
### http://stackoverflow.com/a/33571117
def json_load_byteified(json_text):
    return _byteify(
        json.load(json_text, object_hook=_byteify),
        ignore_dicts=True
    )

def _byteify(data, ignore_dicts=False):
    # if this is a unicode string, return its string representation
    if isinstance(data, unicode):
        return data.encode('utf-8')
    # if this is a list of values, return list of byteified values
    if isinstance(data, list):
        return [_byteify(item, ignore_dicts=True) for item in data]
    # if this is a dictionary, return dictionary of byteified keys and values
    # but only if we haven't already byteified it
    if isinstance(data, dict) and not ignore_dicts:
        return {
            _byteify(key, ignore_dicts=True): _byteify(value, ignore_dicts=True)
            for key, value in data.iteritems()
        }
    # if it's anything else, return it in its original form
    return data
################################


class IntegrationTest(unittest.TestCase):
    def setUp(self):
        self.tmpdir = tempfile.mkdtemp(prefix='summarize_test_')
        os.chdir(self.tmpdir)

    def tearDown(self):
        shutil.rmtree(self.tmpdir)

    def test_main(self):
        def smear(l):
            "given a list of dictionary deltas, return a list of dictionaries"
            cur = {}
            out = []
            for delta in l:
                cur.update(delta)
                out.append(dict(cur))
            return out
        json.dump(smear([
            {'started': 1234, 'number': 1, 'tests_failed': 1, 'tests_run': 2,
             'elapsed': 4, 'path': 'gs://logs/some-job/1', 'job': 'some-job', 'result': 'SUCCESS'},
            {'number': 2, 'path': 'gs://logs/some-job/2'},
            {'number': 3, 'path': 'gs://logs/some-job/3'},
            {'number': 4, 'path': 'gs://logs/some-job/4'},
            {'number': 5, 'path': 'gs://logs/other-job/5', 'job': 'other-job', 'elapsed': 8},
            {'number': 7, 'path': 'gs://logs/other-job/7', 'result': 'FAILURE'},
        ]), open('builds.json', 'w'))
        tests = smear([
            {'name': 'example test', 'build': 'gs://logs/some-job/1',
             'failure_text': 'some awful stack trace exit 1'},
            {'build': 'gs://logs/some-job/2'},
            {'build': 'gs://logs/some-job/3'},
            {'build': 'gs://logs/some-job/4'},
            {'name': 'another test', 'failure_text': 'some other error message'},
            {'name': 'unrelated test', 'build': 'gs://logs/other-job/5'},
            {},  # intentional dupe
            {'build': 'gs://logs/other-job/7'},
        ])
        with open('tests.json', 'w') as f:
            for t in tests:
                f.write(json.dumps(t) + '\n')
        json.dump({
            'node': ['example']
        }, open('owners.json', 'w'))
        summarize.main(summarize.parse_args(
            ['builds.json', 'tests.json',
             '--output_slices=failure_data_PREFIX.json',
             '--owners=owners.json']))
        output = json_load_byteified(open('failure_data.json'))

        # uncomment when output changes
        # import pprint; pprint.pprint(output)

        self.assertEqual(
            output['builds'],
            {'cols': {'elapsed': [8, 8, 4, 4, 4, 4],
                      'executor': [None, None, None, None, None, None],
                      'pr': [None, None, None, None, None, None],
                      'result': ['SUCCESS',
                                 'FAILURE',
                                 'SUCCESS',
                                 'SUCCESS',
                                 'SUCCESS',
                                 'SUCCESS'],
                      'started': [1234, 1234, 1234, 1234, 1234, 1234],
                      'tests_failed': [1, 1, 1, 1, 1, 1],
                      'tests_run': [2, 2, 2, 2, 2, 2]},
             'job_paths': {'other-job': 'gs://logs/other-job',
                           'some-job': 'gs://logs/some-job'},
             'jobs': {'other-job': {'5': 0, '7': 1}, 'some-job': [1, 4, 2]}})

        random_hash_1 = output['clustered'][0]['id']
        random_hash_2 = output['clustered'][1]['id']

        self.assertEqual(
            output['clustered'],
            [{'id': random_hash_1,
              'key': 'some awful stack trace exit 1',
              'tests': [{'jobs': [{'builds': [4, 3, 2, 1],
                                   'name': 'some-job'}],
                         'name': 'example test'}],
              'spans': [29],
              'owner': 'node',
              'text': 'some awful stack trace exit 1'},
             {'id': random_hash_2,
              'key': 'some other error message',
              'tests': [{'jobs': [{'builds': [7, 5],
                                   'name': 'other-job'}],
                         'name': 'unrelated test'},
                        {'jobs': [{'builds': [4], 'name': 'some-job'}],
                         'name': 'another test'}],
              'spans': [24],
              'owner': 'testing',
              'text': 'some other error message'}]
        )

        slice_output = json_load_byteified(open('failure_data_%s.json' % random_hash_1[:2]))

        self.assertEqual(slice_output['clustered'], [output['clustered'][0]])
        self.assertEqual(slice_output['builds']['cols']['started'], [1234, 1234, 1234, 1234])


if __name__ == '__main__':
    unittest.main()
