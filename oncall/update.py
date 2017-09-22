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

"""
Extend on-call rotations fairly using linear programming.
"""

import argparse
import datetime
import glob
import logging
import os
import re
import sys

import yaml
import pulp  # pylint: disable=import-error


TIME_FORMAT = '%b %d %H:%M %Y'  # Jan 03 15:05 2017
SHIFT_FORMAT = '%a %H:%M'       # Mon 14:00


def load_oncall(fname):
    data = yaml.load(open(fname))

    shifts = []
    for shift in data.get('shifts', []):
        match = re.match(r'^([^|]*?)(?:\s*\|\s*(\S*))(?:\s*//\s*(.*))?', shift)
        date, assigned, comment = match.groups()
        date = datetime.datetime.strptime(date, TIME_FORMAT)
        assigned = assigned.split(',')
        shifts.append((date, assigned, comment))

    data['shifts'] = shifts
    return data


def write_oncall(fname, data):
    shifts, data['shifts'] = data['shifts'], []
    for date, assigned, comment in shifts:
        line = '%s | %s' % (date.strftime(TIME_FORMAT), ','.join(assigned))
        if comment:
            line = '%s // %s' % (line.ljust(32), comment)
        data['shifts'].append(line)
    serialized = yaml.dump(data, width=300, default_flow_style=False)
    open(fname, 'w').write(serialized)


def score(shifts, ideal_gap):
    '''
    Compute how much "badness" a given plan of shifts has.

    - Avoid having shifts.
    - Having shifts too close together.
    - Having shifts too far apart.
    - Being a martyr.

    Yes, these are contradictory.
    '''
    # TODO(rmmh): incorporate vacations somehow
    hour_cost = sum(shifts) * 100
    gap_cost = 0
    last_shift = -1

    def calculate_gap_cost(n):
        dist_from_ideal = abs(ideal_gap - n)
        return 10 * dist_from_ideal ** 2

    for n, has_shift in enumerate(shifts):
        if has_shift:
            if last_shift != -1:
                gap_cost += calculate_gap_cost(n - last_shift)
            last_shift = n
    gap_cost += calculate_gap_cost(len(shifts) - last_shift)

    return int((hour_cost + gap_cost) ** 1.2)


def next_shift_change(previous, switch_dates):
    next_date = previous + datetime.timedelta(days=30)
    for switch in switch_dates:
        day_shift = switch.weekday() - previous.weekday()
        if day_shift <= 0:
            day_shift += 7
        switch_date = previous + datetime.timedelta(days=day_shift)
        switch_date = switch_date.replace(hour=switch.hour, minute=switch.minute)
        next_date = min(next_date, switch_date)
    return next_date


def calculate_new_shift_dates(last_shift, extend_upto, shift_switch_dates):
    new_shift_dates = []
    while last_shift <= extend_upto:
        last_shift = next_shift_change(last_shift, shift_switch_dates)
        new_shift_dates.append(last_shift)
    return new_shift_dates


def optimize_future_shifts(new_shift_dates, past_shift_assignees, victims):
    possible_schedules = []
    comb_format = '{:0%db}' % len(new_shift_dates)
    for victim in victims:
        past_shifts = [int(victim in a) for a in past_shift_assignees]
        for n in xrange(2 ** len(new_shift_dates)):
            possible_schedules.append(
                (victim, tuple(past_shifts + map(int, comb_format.format(n)))))

    model = pulp.LpProblem("Shift Scheduling Model", pulp.LpMinimize)
    # pylint: disable=invalid-name
    x = pulp.LpVariable.dicts('shift', possible_schedules, 0, 1, cat=pulp.LpInteger)

    model += sum([score(sched, len(victims)) * x[(victim, sched)]
                  for victim, sched in possible_schedules])

    # each person must have one shift schedule
    for victim in victims:
        model += sum([x[sched] for sched in possible_schedules
                      if victim == sched[0]]) == 1, "Unique shifts for %s" % victim

    # only one person is on for each shift
    for n in xrange(len(new_shift_dates)):
        model += sum([x[sched] for sched in possible_schedules
                      if sched[1][len(past_shift_assignees) + n]]) == 1,\
                 "One person on for future #%d" % n

    model.solve()

    chosen = [[] for _ in xrange(len(new_shift_dates))]
    for sched in possible_schedules:
        if x[sched].value() == 1:
            for n in xrange(len(new_shift_dates)):
                if sched[1][n + len(past_shift_assignees)]:
                    chosen[n].append(sched[0])

    return zip(new_shift_dates, chosen)


def update(data):
    options = data['options']
    shift_switch_dates = [
        datetime.datetime.strptime(s, SHIFT_FORMAT)
        for s in options['shift_switch_dates'].split(',')]

    extend_upto = options.get('extend_upto', '6w')
    if extend_upto.endswith('w'):
        extend_upto = datetime.timedelta(weeks=int(extend_upto[:-1]))
    extend_upto += datetime.datetime.now()

    shifts = list(data['shifts'])  # copy
    last_shift = shifts[-1][0] if shifts else datetime.datetime.now()
    new_shift_dates = calculate_new_shift_dates(last_shift, extend_upto, shift_switch_dates)

    if len(new_shift_dates) < options.get('extend_at_least', 1):
        return data  # bail early if there's no work to do

    plan = optimize_future_shifts(new_shift_dates, [s[1] for s in shifts[-16:]], options['victims'])

    for date, assigned in plan:
        shifts.append((date, assigned, None))

    new_data = dict(data)  # shallow copy
    new_data['shifts'] = shifts
    return new_data


def main(args):
    parser = argparse.ArgumentParser()
    parser.add_argument('files', nargs='*')
    options = parser.parse_args(args)

    if not options.files:
        options.files = glob.glob(os.path.dirname(os.path.abspath(__file__)) + '/*.yaml')

    failed = False
    for path in options.files:
        try:
            data = load_oncall(path)
            new_data = update(data)
            if new_data != data:
                assert 'shifts' in new_data
                write_oncall(path, new_data)
        except:  # pylint: disable=bare-except
            logging.exception('unable to update data from %s', path)
            failed = True
    return failed


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
