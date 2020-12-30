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


import os


DUMP = 'dump.txt'
THREADS = 32
MAX_BAD_RECORDS = 1000
DAYS_OLD = 1.9
DAY = 1
WEEK = 7
MONTH = 30

def print_dump(file):
    if os.path.exists(file):
        with open(file, "r") as fp:
            output = fp.read()
        os.remove(file)
        print(output)
    else:
        print(f'unable to find dump file: {file}')


def call(cmd):
    print('+', cmd)
    status = os.system(cmd)
    if status:
        raise OSError('invocation failed')


def main():
    call(f'time python3 make_db.py --buckets buckets.yaml --junit --threads {THREADS}')

    bq_cmd = f'bq load --source_format=NEWLINE_DELIMITED_JSON --max_bad_records={MAX_BAD_RECORDS}'
    mj_cmd = 'pypy3 make_json.py'

    mj_ext = ''
    bq_ext = ''
    try:
        call(f'{mj_cmd} --days 1 --assert-oldest {DAYS_OLD}')
    except OSError:
        # cycle daily/weekly tables
        bq_ext = ' --replace'
        mj_ext = ' --reset-emitted'

    if os.getenv('DEPLOYMENT', 'staging') == "prod":
        call(f'{mj_cmd} {mj_ext} --days {DAY} | pv | gzip > build_day.json.gz')
        call(f'{bq_cmd} {bq_ext} k8s-gubernator:build.day build_day.json.gz schema.json')

        call(f'{mj_cmd} {mj_ext} --days {WEEK} | pv | gzip > build_week.json.gz')
        call(f'{bq_cmd} {bq_ext} k8s-gubernator:build.week build_week.json.gz schema.json')

        # TODO: (MushuEE) #20024, remove 30 day limit once issue with all uploads is found
        call(f'{mj_cmd} --days {MONTH} | pv | gzip > build_all.json.gz')
        call(f'{bq_cmd} k8s-gubernator:build.all build_all.json.gz schema.json')

        call(f'python3 stream.py --poll kubernetes-jenkins/gcs-changes/kettle ' \
            '--dataset k8s-gubernator:build --tables all:{MONTH} day:{DAY} week:{WEEK} --stop_at=1')
    else:
        call(f'{mj_cmd} | pv | gzip > build_staging.json.gz')
        call(f'{bq_cmd} k8s-gubernator:build.staging build_staging.json.gz schema.json')
        call('python3 stream.py --poll kubernetes-jenkins/gcs-changes/kettle-staging ' \
            '--dataset k8s-gubernator:build --tables staging:0 --stop_at=1')

if __name__ == '__main__':
    os.chdir(os.path.dirname(__file__))
    os.environ['TZ'] = 'America/Los_Angeles'
    main()
