#!/usr/bin/python3

# Copyright 2018 The Kubernetes Authors.
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

"""Parser for e2e test logs.

Useful for finding which tests overlapped with a certain event.
"""

import argparse
import datetime
import re


_LINE_RE = re.compile(r'^[IWE]111[45] \d\d:\d\d:\d\d\.\d\d\d\] ?(.*)')
_DATE_FORMAT = '%Y-%m-%d %H:%M:%S'
_CURRENT_YEAR = datetime.datetime.utcnow().year


class TestOutput:
    def __init__(self):
        self._lines = []
        self._start = None
        self._end = None
        self._it = None

    def append(self, line):
        self._lines.append(line)
        try:
            timestamp = datetime.datetime.strptime(line[:19], '%b %d %H:%M:%S.%f').replace(
                year=_CURRENT_YEAR)
        except:  # pylint: disable=bare-except
            pass
        else:
            if not self._start:
                self._start = timestamp
            self._end = timestamp
        if line.startswith('[It] '):
            self._it = line
        if line.startswith('[BeforeEach] ') and not self._it:
            self._it = line

    def overlaps(self, after, before):
        if self._end and after and self._end < after:
            return False
        if self._start and before and self._start > before:
            return False
        return True

    def __len__(self):
        return len(self._lines)

    def __str__(self):
        if not self._lines:
            return '<empty>'
        return 'Test %s->%s (%5d lines) %s' % (
            self._start, self._end, len(self), self._it if self._it else '')


def _get_tests(log):
    current_test = TestOutput()
    for line in log:
        line = line.rstrip()
        match = _LINE_RE.match(line)
        if not match:
            raise Exception('line %s does not match' % line)
        if '------------------------------' in line:
            ended_test = current_test
            current_test = TestOutput()
            if len(ended_test) <= 1:
                continue
            yield ended_test
        else:
            current_test.append(match.group(1))
    yield current_test


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--log_year', default=_CURRENT_YEAR,
                        help=('Year in which the log was created. '
                              'Needed because the year is omitted in the log.'))
    parser.add_argument('--after',
                        help=('Show tests which ended at or after this time '
                              '(format: %s).' % _DATE_FORMAT.replace('%', '%%')))
    parser.add_argument('--before',
                        help=('Show tests which started at or before this time '
                              '(format: %s).' % _DATE_FORMAT.replace('%', '%%')))
    parser.add_argument('file')

    args = parser.parse_args()
    after = datetime.datetime.strptime(args.after, _DATE_FORMAT) if args.after else None
    before = datetime.datetime.strptime(args.before, _DATE_FORMAT) if args.before else None
    if after and before and after.year != before.year:
        raise Exception('Logs spanning year boundary are not supported.')
    if not args.log_year and (after or before):
        year = after.year if after else before.year
        if year != _CURRENT_YEAR:
            raise Exception('Please explicitly specify the year in which the log was created.')
    with open(args.file) as log:
        for test in _get_tests(log):
            if test.overlaps(after, before):
                print(str(test))


if __name__ == '__main__':
    main()
