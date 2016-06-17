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

from __future__ import division

import collections
import cStringIO
import datetime
import subprocess
import sys
import time
import traceback

import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.dates as mdates


def render(history_lines, out_file):
    """Read historical data and save to out_file as png."""
    dts = []
    prs = []
    queued = []
    instant_happiness = []
    daily_happiness = []

    blocked_intervals = []
    daily_queue = collections.deque()

    last_blocked = None
    daily_sum = 0
    for line in history_lines:
        try:
            date, time, online, pr, queue, run, blocked = line.strip().split(' ')
        except ValueError:  # line does not fit expected criteria
            continue
        pr, queue, run = int(pr), int(queue), int(run)
        dt = datetime.datetime.strptime('{} {}'.format(date, time), '%Y-%m-%d %H:%M:%S.%f')
        if dt < datetime.datetime.now() - datetime.timedelta(days=30):
            continue
        blocked = blocked == 'True'
        happy = online and not blocked

        daily_sum += happy
        daily_queue.append(happy)
        if len(daily_queue) > 60*24:
            daily_sum -= daily_queue.popleft()
        if not online:
            continue

        happiness = daily_sum / len(daily_queue)
        # Make them steps instead of slopes.
        if len(dts) > 0:
            dts.append(dt)
            prs.append(prs[-1])
            queued.append(queued[-1])
            instant_happiness.append(happy)
            daily_happiness.append(happiness)
        dts.append(dt)
        prs.append(pr)
        queued.append(queue)
        instant_happiness.append(happy)
        daily_happiness.append(happiness)

        if last_blocked is None and blocked:
            last_blocked = dt
        elif last_blocked is not None and not blocked:
            blocked_intervals.append((last_blocked, dt))
            last_blocked = None
    if last_blocked is not None:
        blocked_intervals.append((last_blocked, dt))

    fig, (ax1, ax2, ax3) = plt.subplots(3, sharex=True, figsize=(16, 8), dpi=100)

    ax1.plot(dts, prs)
    ax2.plot(dts, queued)
    ax3.plot(dts, daily_happiness)

    plt.gca().xaxis.set_major_formatter(mdates.DateFormatter('%m/%d/%Y'))
    plt.gca().xaxis.set_major_locator(mdates.DayLocator())

    ax1.set_ylabel('Pull Requests')
    ax2.set_ylabel('Submit Queue Size')
    ax3.set_ylabel('Merge Health Over Last Day')


    ax3.set_ylim([0.0, 1.0])
    ax3.set_xlim(left=datetime.datetime.now() - datetime.timedelta(days=14))

    fig.autofmt_xdate()

    for start, end in blocked_intervals:
        for plot in [ax2, ax3]:
            plot.axvspan(start, end, alpha=0.3, color='red', linewidth=0)


    last_week = datetime.datetime.now() - datetime.timedelta(days=7)
    week_happy, week_points = 0, 0
    for dt, instant in zip(dts, instant_happiness):
        if dt < last_week:
            continue
        if instant:
            week_happy += 1
        week_points += 1

    if week_points > 0:
        fig.text(.1, .08, 'Health for the last week: %.0f%%' % (100.0 * week_happy / week_points))

    plt.savefig(out_file, bbox_inches='tight')


def render_forever(history_uri, png_uri):
    """Download results from history_uri, render to png and save to png_uri."""
    buf = cStringIO.StringIO()
    while True:
        print >>sys.stderr, 'Truncate render buffer'
        buf.seek(0)
        buf.truncate()
        print >>sys.stderr, 'Cat latest results from %s...' % history_uri
        try:
            history = subprocess.check_output(['gsutil', 'cat', history_uri])
        except subprocess.CalledProcessError:
            traceback.print_exc()
            time.sleep(10)
            continue

        print >>sys.stderr, 'Render results to buffer...'
        render(history.split('\n')[-60*24*21:], buf)  # Last 21 days

        print >>sys.stderr, 'Copy buffer to %s...' % png_uri
        proc = subprocess.Popen(
            ['gsutil', '-q', '-h', 'Content-Type:image/png',
             'cp', '-a', 'public-read', '-', png_uri],
            stdin=subprocess.PIPE)
        proc.communicate(buf.getvalue())
        code = proc.wait()
        if code:
          print >>sys.stderr, 'Failed to copy rendering to %s: %d' % (png_uri, code)
        time.sleep(60)



if __name__ == '__main__':
    render_forever(*sys.argv[1:])
