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


import json
import os
import sqlite3
import time
import zlib


class Database:
    """
    Store build and test result information, and support incremental updates to results.
    """

    DEFAULT_INCREMENTAL_TABLE = 'build_emitted'

    def __init__(self, path=None):
        if path is None:
            path = os.getenv('KETTLE_DB') or 'build.db'
        self.db = sqlite3.connect(path)
        self.db.executescript('''
            create table if not exists build(gcs_path primary key, started_json, finished_json, finished_time);
            create table if not exists file(path string primary key, data);
            create table if not exists build_junit_missing(build_id integer primary key);
            create index if not exists build_finished_time_idx on build(finished_time)
            ''')

    def commit(self):
        self.db.commit()

    def get_existing_builds(self, jobs_dir):
        """
        Return a set of (job, number) tuples indicating already present builds.

        A build is already present if it has a finished.json, or if it's older than
        five days with no finished.json.
        """
        builds_have_paths = self.db.execute(
            'select gcs_path from build'
            ' where gcs_path between ? and ?'
            ' and finished_json IS NOT NULL'
            ,
            (jobs_dir + '\x00', jobs_dir + '\x7f')).fetchall()
        path_tuple = lambda path: tuple(path[len(jobs_dir):].split('/')[-2:])
        builds_have = {path_tuple(path) for (path,) in builds_have_paths}
        for path, started_json in self.db.execute(
                'select gcs_path, started_json from build'
                ' where gcs_path between ? and ?'
                ' and started_json IS NOT NULL and finished_json IS NULL',
                (jobs_dir + '\x00', jobs_dir + '\x7f')):
            started = json.loads(started_json)
            if int(started['timestamp']) < time.time() - 60*60*24*5:
                # over 5 days old, no need to try looking for finished any more.
                builds_have.add(path_tuple(path))
        return builds_have

    ### make_db

    def insert_build(self, build_dir, started, finished):
        """
        Add a build with optional started and finished dictionaries to the database.
        """
        started_json = started and json.dumps(started, sort_keys=True)
        finished_json = finished and json.dumps(finished, sort_keys=True)
        if not self.db.execute(
                'select 1 from build where gcs_path=? '
                'and started_json=? and finished_json=?',
                (build_dir, started_json, finished_json)).fetchone():
            rowid = self.db.execute(
                'insert or replace into build values(?,?,?,?)',
                (build_dir, started_json, finished_json,
                 finished and finished.get('timestamp', None))).lastrowid
            self.db.execute('insert into build_junit_missing values(?)', (rowid,))
            return True
        return False

    def get_builds_missing_junit(self):
        """
        Return (rowid, path) for each build that hasn't enumerated junit files.
        """
        # cleanup
        self.db.execute('delete from build_junit_missing'
                        ' where build_id not in (select rowid from build)')
        return self.db.execute(
            'select rowid, gcs_path from build'
            ' where rowid in (select build_id from build_junit_missing)'
        ).fetchall()

    def insert_build_junits(self, build_id, junits):
        """
        Insert a junit dictionary {gcs_path: contents} for a given build's rowid.
        """
        for path, data in junits.items():
            self.db.execute('replace into file values(?,?)',
                            (path, memoryview(zlib.compress(data.encode('utf-8'), 9))))
        self.db.execute('delete from build_junit_missing where build_id=?', (build_id,))

    ### make_json

    def _init_incremental(self, table):
        """
        Create tables necessary for storing incremental emission state.
        """
        self.db.execute('create table if not exists %s(build_id integer primary key, gen)' % table)

    @staticmethod
    def _get_builds(results):
        for rowid, path, started, finished in results:
            started = json.loads(started) if started else started
            finished = json.loads(finished) if finished else finished
            yield rowid, path, started, finished

    def get_builds(self, path='', min_started=0, incremental_table=DEFAULT_INCREMENTAL_TABLE):
        """
        Iterate through (buildid, gcs_path, started, finished) for each build under
        the given path that has not already been emitted.

        Args:
            path (string, optional): build path to fetch
            min_started (int, optional): epoch time to fetch builds since
            incremental_table (string, optional): table name

        Returns:
            Generator containing rowID, path, and dicts representing the started and finished json
        """
        self._init_incremental(incremental_table)
        results = self.db.execute(
            'select rowid, gcs_path, started_json, finished_json from build '
            'where gcs_path like ?'
            ' and finished_time >= ?' +
            ' and rowid not in (select build_id from %s)'
            ' order by finished_time' % incremental_table
            , (path + '%', min_started)).fetchall()
        return self._get_builds(results)

    def get_builds_from_paths(self, paths, incremental_table=DEFAULT_INCREMENTAL_TABLE):
        self._init_incremental(incremental_table)
        results = self.db.execute(
            'select rowid, gcs_path, started_json, finished_json from build '
            'where gcs_path in (%s)'
            ' and rowid not in (select build_id from %s)'
            ' order by finished_time' % (','.join(['?'] * len(paths)), incremental_table)
            , paths).fetchall()
        return self._get_builds(results)

    def test_results_for_build(self, path):
        """
        Return a list of file data under the given path. Intended for JUnit artifacts.
        """
        results = []
        for dataz, in self.db.execute(
                'select data from file where path between ? and ?',
                (path, path + '\x7F')):
            data = zlib.decompress(dataz).decode('utf-8')
            if data:
                results.append(data)
        return results

    def get_oldest_emitted(self, incremental_table):
        return self.db.execute('select min(finished_time) from build '
                               'where rowid in (select build_id from %s)'
                               % incremental_table).fetchone()[0]

    def reset_emitted(self, incremental_table=DEFAULT_INCREMENTAL_TABLE):
        self.db.execute('drop table if exists %s' % incremental_table)

    def insert_emitted(self, rows_emitted, incremental_table=DEFAULT_INCREMENTAL_TABLE):
        self._init_incremental(incremental_table)
        gen, = self.db.execute('select max(gen)+1 from %s' % incremental_table).fetchone()
        if not gen:
            gen = 0
        self.db.executemany(
            'insert into %s values(?,?)' % incremental_table,
            ((row, gen) for row in rows_emitted))
        self.db.commit()
        return gen
