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
matplotlib.use('Agg')  # For savefig
import matplotlib.dates as mdates
import matplotlib.patches as mpatches
import matplotlib.pyplot as plt
import numpy

from pprint import pprint

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

def fresh_color(dt):
    if datetime.datetime.utcnow() - dt < datetime.timedelta(hours=1):
        return 'black'
    return 'r'


def merges_color(merges):
    if merges > 30:
        return 'g'
    if merges > 15:
        return 'y'
    return 'r'


def happy_color(health):
    if health > 0.8:
        return 'g'
    if health > 0.6:
        return 'y'
    return 'r'


def depth_color(depth):
    if depth < 20:
        return 'g'
    if depth < 40:
        return 'y'
    return 'r'


def wait_color(delta):
    if delta < datetime.timedelta(hours=4):
        return 'g'
    if delta < datetime.timedelta(hours=24):
        return 'y'
    return 'r'


def format_timedelta(delta):
    return '%dd%dh%dm' % (
        delta.days, delta.seconds / 3600, (delta.seconds % 3600) / 60)


class Sampler(object):
    mean = 0
    total = 0

    def __init__(self, maxlen=60*24):
        self.maxlen = maxlen
        self.samples = collections.deque()

    def __iadd__(self, sample):
        self.samples.append(sample)
        self.total += sample
        while len(self.samples) > self.maxlen:
            self.total -= self.samples.popleft()
        self.mean = float(self.total) / len(self.samples)
        return self


def render(history_lines, out_file):
    """Read historical data and save to out_file as img."""
    dts = []
    prs = []
    queued = []
    daily_happiness = []  # Percentage of last day queue was not blocked
    merge_rate = []  # Merge rate for the past 24 active hours
    real_merge_rate = []  # Merge rate including when queue is empty
    merges = []

    blocked_intervals = []
    offline_intervals = []

    active_merges = Sampler()
    real_merges = Sampler()
    happy_moments = Sampler()

    daily_merged = collections.deque()
    actually_merged = collections.deque()

    dt = None
    start_blocked = None
    start_offline = None
    last_merge = 0  # Number of merges last sample, resets on queue restart

    for line in history_lines:
        try:
            dt, online, pr, queue, _, blocked, merged = parse_line(
                *line.strip().split(' '))
        except TypeError:  # line does not fit expected criteria
            continue
        if dt < datetime.datetime.now() - datetime.timedelta(days=30):
            continue
        if not pr and not queue and not merged:  # Bad sample
            continue

        if merged >= last_merge:
            did_merge = merged - last_merge
        elif online:  # Restarts reset the number to 0
            did_merge = merged
        else:
            did_merge = 0

        last_merge = merged
        happy_moments += int(bool(online and not blocked))

        real_merges += did_merge
        if queue or did_merge:  # Only add samples when queue is busy.
            active_merges += did_merge

        if not start_offline and not online:
            start_offline = dt
        if start_offline and online:
            offline_intervals.append((start_offline, dt))
            start_offline = None

        if not online:  # Skip offline entries
            continue

        # Make them steps instead of slopes.
        if dts:
            dts.append(dt)

            # Append the previous value at the current time
            # which makes all changes move at right angles.
            daily_happiness.append(daily_happiness[-1])
            merge_rate.append(merge_rate[-1])
            merges.append(did_merge)
            prs.append(prs[-1])
            queued.append(queued[-1])
            real_merge_rate.append(real_merge_rate[-1])
        dts.append(dt)

        daily_happiness.append(happy_moments.mean)
        merge_rate.append(active_merges.total)
        merges.append(did_merge)
        prs.append(pr)
        queued.append(queue)
        real_merge_rate.append(real_merges.total)

        if not start_blocked and blocked:
            start_blocked = dt
        if start_blocked and not blocked:
            blocked_intervals.append((start_blocked, dt))
            start_blocked = None
    if start_blocked:
        blocked_intervals.append((start_blocked, dt))
    if start_offline:
        offline_intervals.append((start_offline, dt))

    fig, (ax_open, ax_merged, ax_health) = plt.subplots(
        3, sharex=True, figsize=(16, 8), dpi=100)
    ax_queued = ax_open.twinx()
    ax_merged.yaxis.tick_right()
    ax_merged.yaxis.set_label_position('right')
    ax_health.yaxis.tick_right()
    ax_health.yaxis.set_label_position('right')

    ax_open.plot(dts, prs, 'b-')
    merge_color = merges_color(merge_rate[-1])
    p_merge, = ax_merged.plot(dts, merge_rate, '%s-' % merge_color)
    p_real_merge, = ax_merged.plot(dts, real_merge_rate, '%s:' % merge_color, alpha=0.5)

    health_color = happy_color(daily_happiness[-1])
    health_line = '%s-' % health_color
    ax_health.plot(dts, daily_happiness, health_line)

    ax_queued.plot(dts, queued, '%s-' % depth_color(queued[-1]))

    plt.gca().xaxis.set_major_formatter(mdates.DateFormatter('%m/%d/%Y'))
    plt.gca().xaxis.set_major_locator(mdates.DayLocator())

    ax_open.set_ylabel('Open PRs: %d' % prs[-1], color='b')
    ax_queued.set_ylabel(
        'Queued PRs: %d' % queued[-1],
        color=depth_color(queued[-1]))

    ax_health.set_ylabel(
        'Queue health: %.2f' % daily_happiness[-1],
        color=health_color)

    ax_merged.set_ylabel('Merge capacity: %d/d' % merge_rate[-1], color=merge_color)

    ax_health.set_ylim([0.0, 1.0])
    ax_health.set_xlim(left=datetime.datetime.now() - datetime.timedelta(days=21))

    fig.autofmt_xdate()

    for start, end in offline_intervals:
        ax_merged.axvspan(start, end, alpha=0.2, color='black', linewidth=0)
        ax_health.axvspan(start, end, alpha=0.2, color='black', linewidth=0)

    for start, end in blocked_intervals:
        ax_health.axvspan(start, end, alpha=0.2, color='brown', linewidth=0)

    p_blocked = mpatches.Patch(color='brown', label='blocked', alpha=0.2)
    p_offline = mpatches.Patch(color='black', label='offline', alpha=0.2)
    ax_health.legend([p_offline, p_blocked], ['offline', 'blocked'], 'lower left', fontsize='x-small')
    ax_merged.legend([p_merge, p_real_merge, p_offline], ['capacity', 'actual', 'offline'], 'lower left', fontsize='x-small')

    last_week = datetime.datetime.now() - datetime.timedelta(days=6)

    halign = 'center'
    xpos = 0.5
    fig.text(
        xpos, 0.08, 'Weekly statistics', horizontalalignment=halign)

    weekly_merge_rate = numpy.mean([
        m for (d, m) in zip(dts, merge_rate) if d >= last_week])
    weekly_merges = sum(
        m for (d, m) in zip(dts, merges) if d >= last_week)
    weekly_merges /= 2  # Due to steps

    fig.text(
        xpos, .00,
        'Merge capacity: %.1f PRs/day (merged %d)' % (
            weekly_merge_rate, weekly_merges),
        color=merges_color(weekly_merge_rate),
        horizontalalignment=halign,
    )

    week_happiness = numpy.mean(
        [h for (d, h) in zip(dts, daily_happiness) if d >= last_week])
    fig.text(
        xpos, .04,
        'Unblocked %.1f%% of this week' % (100 * week_happiness),
        color=happy_color(week_happiness),
        horizontalalignment=halign,
    )

    if not queued[-1]:
      delta = datetime.timedelta(0)
      wait = 'clear'
    elif not merge_rate[-1]:
      delta = datetime.timedelta(days=90)
      wait = 'forever'
    else:
      delta = datetime.timedelta(float(queued[-1]) / merge_rate[-1])
      wait = format_timedelta(delta)
    fig.text(
        xpos, -0.04,
        'Queue backlog: %s' % wait,
        color=wait_color(delta),
        horizontalalignment=halign,
    )

    if dt:
        fig.text(
            0.1, -0.04,
            'image: %s, sample: %s' % (
                datetime.datetime.utcnow().strftime('%Y-%m-%d %H:%M'),
                dt.strftime('%Y-%m-%d %H:%M'),
            ),
            horizontalalignment='left',
            fontsize='x-small',
            color=fresh_color(dt),
        )

    plt.savefig(out_file, bbox_inches='tight', format='svg')
    plt.close()


def render_forever(history_uri, img_uri, service_account=None):
    """Download results from history_uri, render to svg and save to img_uri."""
    if service_account:
      print >>sys.stderr, 'Activating service account using: %s' % service_account
      subprocess.check_call(
          ['gcloud', 'auth', 'activate-service-account', '--key-file=%s' % service_account])
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
             '-h', 'Cache-Control:public, max-age=%d' % (60 if service_account else 5),
             '-h', 'Content-Encoding:gzip',  # GCS decompresses if necessary
             'cp', '-a', 'public-read', '-', img_uri],
            stdin=subprocess.PIPE)
        proc.communicate(buf.getvalue())
        code = proc.wait()
        if code:
          print >>sys.stderr, 'Failed to copy rendering to %s: %d' % (img_uri, code)
        time.sleep(60)



if __name__ == '__main__':
    # log all arguments.
    pprint(sys.argv)

    render_forever(*sys.argv[1:])
