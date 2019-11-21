#!/usr/bin/env python3

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

import unittest

import model


class ModelTest(unittest.TestCase):
    def setUp(self):
        self.db = model.Database(':memory:')

    def test_insert_build(self):
        self.db.insert_build('/some/dir/123', {'timestamp': 123}, {'timestamp': 140})
        self.assertEqual(self.db.get_existing_builds('/some/'), {('dir', '123')})

    def test_insert_junits(self):
        self.db.insert_build('/some/dir/123', {'timestamp': 123}, {'timestamp': 140})
        self.assertEqual(self.db.get_builds_missing_junit(), [(1, '/some/dir/123')])

        self.db.insert_build_junits(1, {'/some/dir/123/foo.txt': 'example'})
        self.assertEqual(self.db.test_results_for_build('/some/dir/123/'), ['example'])

    def test_incremental(self):
        def add_build(num):
            self.db.insert_build(
                '/some/dir/%d' % num, {'timestamp': 123}, {'timestamp': 150})

        def expect(builds):
            rows = []
            have = set()
            for rowid, path, _started, _finished in self.db.get_builds():
                rows.append(rowid)
                have.add(int(path[path.rindex('/', 0, -1) + 1:]))
            self.assertEqual(have, builds)
            self.db.insert_emitted(rows)
            self.assertEqual(have, builds)

        add_build(1)

        expect({1})
        expect(set())

        add_build(2)
        add_build(3)

        expect({2, 3})

        self.db.reset_emitted()

        expect({1, 2, 3})
        expect(set())


if __name__ == '__main__':
    unittest.main()
