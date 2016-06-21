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
import gzip
import os
import subprocess
import sys
import time
import traceback

import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.dates as mdates

def parse_line(
        date, time, online, pr, queue,
        run, blocked, merge_count=0):  # merge_count may be missing
    return (
        datetime.datetime.strptime('{} {}'.format(date, time), '%Y-%m-%d %H:%M:%S.%f'),
        online == 'True',  # Merge queue is down/initializing
        int(pr),  # Number of open PRs
        int(queue),  # PRs in the queue
        int(run),  # Totally worthless
        blocked == 'True',  # Cannot merge
        int(merge_count),  # Number of merges
    )


def merges_color(merges):
    if merges > 15:
        return 'g'
    if merges > 5:
        return 'y'
    return 'r'


def happy_color(health):
    if health > 0.8:
        return 'g'
    if health > 0.6:
        return 'y'
    return 'r'


def render(history_lines, out_file):
    """Read historical data and save to out_file as img."""
    dts = []
    prs = []
    queued = []
    instant_happiness = []
    daily_happiness = []
    daily_merges = []

    blocked_intervals = []
    offline_intervals = []
    daily_queue = collections.deque()
    daily_merged = collections.deque()

    last_blocked = None
    last_offline = None
    happy_sum = 0
    merge_sum = 0
    last_merge = 0

    for line in history_lines:
        try:
            dt, online, pr, queue, _, blocked, merged = parse_line(
                *line.strip().split(' '))
        except TypeError:  # line does not fit expected criteria
            continue
        if dt < datetime.datetime.now() - datetime.timedelta(days=30):
            continue
        happy = online and not blocked
        happy_sum += happy

        did_merge = last_merge and merged != last_merge
        last_merge = merged
        merge_sum += did_merge

        daily_queue.append(happy)
        daily_merged.append(did_merge)
        if len(daily_queue) > 60*24:
            happy_sum -= daily_queue.popleft()
            merge_sum -= daily_merged.popleft()

        if not last_offline and not online:
            last_offline = dt
        if last_offline and online:
            offline_intervals.append((last_offline, dt))
            last_offline = None

        if not online:  # Skip offline entries
            continue

        happiness = happy_sum / len(daily_queue)
        # Make them steps instead of slopes.
        if dts:
            dts.append(dt)
            prs.append(prs[-1])
            queued.append(queued[-1])
            instant_happiness.append(happy)
            daily_happiness.append(happiness)
            daily_merges.append(merge_sum)
        dts.append(dt)
        prs.append(pr)
        queued.append(queue)
        instant_happiness.append(happy)
        daily_happiness.append(happiness)
        daily_merges.append(merge_sum)

        if not last_blocked and blocked:
            last_blocked = dt
        if last_blocked and not blocked:
            blocked_intervals.append((last_blocked, dt))
            last_blocked = None
    if last_blocked:
        blocked_intervals.append((last_blocked, dt))
    if last_offline:
        offline_intervals.append((last_offline, dt))

    fig, (ax_open, ax_offline, ax_blocked) = plt.subplots(
        3, sharex=True, figsize=(16, 8), dpi=100)
    ax_merged = ax_open.twinx()
    ax_health = ax_blocked.twinx()
    ax_queued = ax_offline.twinx()

    ax_open.plot(dts, prs, 'b-')
    merge_color = merges_color(daily_merges[-1])
    ax_merged.plot(dts, daily_merges, '%s-' % merge_color)

    health_color = happy_color(daily_happiness[-1])
    health_line = '%s-' % health_color
    ax_health.plot(dts, daily_happiness, health_line)
    ax_blocked.plot(dts, daily_happiness, health_line)

    ax_queued.plot(dts, queued, 'b-')
    ax_offline.plot(dts, queued, 'b-')


    plt.gca().xaxis.set_major_formatter(mdates.DateFormatter('%m/%d/%Y'))
    plt.gca().xaxis.set_major_locator(mdates.DayLocator())

    ax_open.set_ylabel('Open pull requests', color='b')
    ax_merged.set_ylabel('Merges: %d/d' % daily_merges[-1], color=merge_color)

    ax_blocked.set_ylabel('Queue blocked', color='brown')
    ax_health.set_ylabel(
        'Queue health: %.2f' % daily_happiness[-1],
        color=health_color)

    ax_offline.set_ylabel('Queue offline')
    ax_queued.set_ylabel('Queued PRs: %d' % queued[-1], color='b')


    ax_health.set_ylim([0.0, 1.0])
    ax_health.set_xlim(left=datetime.datetime.now() - datetime.timedelta(days=21))

    fig.autofmt_xdate()

    for start, end in blocked_intervals:
        ax_health.axvspan(start, end, alpha=0.2, color='brown', linewidth=0)

    for start, end in offline_intervals:
        ax_queued.axvspan(start, end, alpha=0.2, color='black', linewidth=0)
        ax_health.axvspan(start, end, alpha=0.2, color='black', linewidth=0)


    last_week = datetime.datetime.now() - datetime.timedelta(days=7)
    week_happy, week_points = 0, 0
    for dt, instant in zip(dts, instant_happiness):
        if dt < last_week:
            continue
        if instant:
            week_happy += 1
        week_points += 1

    if week_points > 0:
        fig.text(
            .5, .08,
            'Weekly submit queue health: %.0f%%' % (100.0 * week_happy / week_points),
            color=happy_color(float(week_happy)/week_points),
            horizontalalignment='center',
        )

    plt.savefig(out_file, bbox_inches='tight', format='svg')
    plt.close()


def render_forever(history_uri, img_uri):
    """Download results from history_uri, render to svg and save to img_uri."""
    buf = cStringIO.StringIO()
    while True:
        print >>sys.stderr, 'Truncate render buffer'
        buf.seek(0)
        buf.truncate()
        print >>sys.stderr, 'Cat latest results from %s...' % history_uri
        try:
            history = subprocess.check_output(
                ['gsutil', '-q', 'cat', history_uri])
        except subprocess.CalledProcessError:
            traceback.print_exc()
            time.sleep(10)
            continue

        print >>sys.stderr, 'Render results to buffer...'
        with gzip.GzipFile(
            os.path.basename(img_uri), mode='wb', fileobj=buf) as compressed:
            render(history.split('\n')[-60*24*21:], compressed)  # Last 21 days

        print >>sys.stderr, 'Copy buffer to %s...' % img_uri
        proc = subprocess.Popen(
            ['gsutil', '-q',
             '-h', 'Content-Type:image/svg+xml',
             '-h', 'Cache-Control:public, max-age=60',
             '-h', 'Content-Encoding:gzip',  # GCS decompresses if necessary
             'cp', '-a', 'public-read', '-', img_uri],
            stdin=subprocess.PIPE)
        proc.communicate(buf.getvalue())
        code = proc.wait()
        if code:
          print >>sys.stderr, 'Failed to copy rendering to %s: %d' % (png_uri, code)
        time.sleep(60)



if __name__ == '__main__':
    render_forever(*sys.argv[1:])
