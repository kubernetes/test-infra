#!/usr/bin/env python

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

"""Read historical samples of merge queue and plot them."""

from __future__ import division

import collections
import cStringIO
import datetime
import gzip
import os
import pprint
import subprocess
import sys
import time
import traceback

# pylint: disable=import-error,wrong-import-position
try:
    import matplotlib
    matplotlib.use('Agg')  # For savefig

    import matplotlib.dates as mdates
    import matplotlib.patches as mpatches
    import matplotlib.pyplot as plt

    import numpy
except ImportError:
    # TODO(fejta): figure out how to add matplotlib and numpy to the bazel
    # workspace. Until then hack around this for unit testing.
    # pylint: disable=invalid-name
    numpy = mdates = mpatches = plt = NotImplementedError
# pylint: enable=wrong-import-position,import-error

DAYS = 21  # Graph this many days of history.

def mean(*a):
    """Calculate the mean for items."""
    return numpy.mean(*a)  # pylint: disable=no-value-for-parameter

def parse_line(
        date, timenow, online, pulls, queue,
        run, blocked, merge_count=0):  # merge_count may be missing
    """Parse a sq/history.txt line."""
    if '.' not in timenow:
        timenow = '%s.0' % timenow
    return (
        datetime.datetime.strptime(
            '%s %s' % (date, timenow),
            '%Y-%m-%d %H:%M:%S.%f'),
        online == 'True',  # Merge queue is down/initializing
        int(pulls),  # Number of open PRs
        int(queue),  # PRs in the queue
        int(run),  # Totally worthless
        blocked == 'True',  # Cannot merge
        int(merge_count),  # Number of merges
    )

def fresh_color(tick):
    """Return pyplot color for freshness of data."""
    if datetime.datetime.utcnow() - tick < datetime.timedelta(hours=1):
        return 'k'  # black
    return 'r'


def merge_color(rate):
    """Return pyplot color for merge rate."""
    if rate < 15:
        return 'r'
    if rate < 30:
        return 'y'
    return 'g'


def backlog_color(backlog):
    """Return pyplot color for queue backlog."""
    if backlog < 5:
        return 'g'
    if backlog > 24:
        return 'r'
    return 'y'


def happy_color(health):
    """Return pyplot color for health percentage."""
    if health > 0.8:
        return 'g'
    if health > 0.6:
        return 'y'
    return 'r'


def depth_color(depth):
    """Return pyplot color for the queue depth."""
    if depth < 20:
        return 'g'
    if depth < 40:
        return 'y'
    return 'r'


def format_timedelta(delta):
    """Return XdYhZm string representing timedelta."""
    return '%dd%dh%dm' % (
        delta.days, delta.seconds / 3600, (delta.seconds % 3600) / 60)


class Sampler(object):  # pylint: disable=too-few-public-methods
    """Track mean and total for a given window of items."""
    mean = 0
    total = 0

    def __init__(self, maxlen=60*24):
        self.maxlen = maxlen
        self.samples = collections.deque()

    def __iadd__(self, sample):
        self.append(sample)
        return self

    def append(self, sample):
        """Append a sample, updating total and mean, dropping old samples."""
        self.samples.append(sample)
        self.total += sample
        while len(self.samples) > self.maxlen:
            self.total -= self.samples.popleft()
        self.mean = float(self.total) / len(self.samples)


class Results(object):  # pylint: disable=too-few-public-methods,too-many-instance-attributes
    """Results processed from sq/history.txt"""
    def __init__(self):
        self.dts = []
        self.prs = []
        self.queued = []
        self.queue_avg = []
        self.happiness = {  # Percenteage of last N days queue was unblocked
            1: [],
            14: [],
        }
        self.active_merge_rate = {  # Merge rate when queue is active
            1: [],
            14: [],
        }
        self.merge_rate = {  # Merge rate including when queue is empty
            1: [],
            14: [],
        }
        self.merges = []
        self.backlog = {  # Queue time in hours during the past N days
            1: [],
            14: [],
        }

        self.blocked_intervals = []
        self.offline_intervals = []

    def append(self, tick, did_merge, pulls, queue,
               real_merges, active_merges, happy_moments):
        """Append a sample of results.

        Args:
          tick: datetime of this sample
          did_merge: number of prs merged
          pulls: number of open prs
          queue: number of approved prs waiting for merge
          real_merges: merge rate over various time periods
          active_merges: merge rate when queue is active (full or merging)
          happy_moments: window of when the queue has been unblocked.
        """
        # pylint: disable=too-many-locals
        # Make them steps instead of slopes.
        if self.dts:
            self.dts.append(tick)

            # Append the previous value at the current time
            # which makes all changes move at right angles.
            for happy in self.happiness.values():
                happy.append(happy[-1])
            self.merges.append(did_merge)  # ???
            self.prs.append(self.prs[-1])
            self.queued.append(self.queued[-1])
            self.queue_avg.append(self.queue_avg[-1])
            for val in self.merge_rate.values():
                val.append(val[-1])
            for val in self.active_merge_rate.values():
                val.append(val[-1])
        self.dts.append(tick)
        for days, happy in self.happiness.items():
            happy.append(happy_moments[days].mean)
        self.merges.append(did_merge)
        self.prs.append(pulls)
        self.queued.append(queue)
        weeks2 = 14*24*60
        avg = mean(self.queued[-weeks2:])
        self.queue_avg.append(avg)
        for days in self.merge_rate:
            self.merge_rate[days].append(real_merges[days].total / float(days))
        for days in self.active_merge_rate:
            self.active_merge_rate[days].append(active_merges[days].total / float(days))
        for dur, items in self.backlog.items():
            dur = 60*24*dur
            if items:
                items.append(items[-1])
            dur_merges = sum(self.merges[-dur:]) * 24.0
            if dur_merges:
                items.append(sum(self.queued[-dur:]) / dur_merges)
            elif items:
                items.append(items[-1])
            else:
                items.append(0)



def output(history_lines, results):  # pylint: disable=too-many-locals,too-many-branches
    """Read historical data and return processed info."""
    real_merges = {
        1: Sampler(),
        14: Sampler(14*60*24),
    }
    active_merges = {
        1: Sampler(),
        14: Sampler(14*60*24),
    }
    happy_moments = {d: Sampler(d*60*24) for d in results.happiness}

    tick = None
    last_merge = 0  # Number of merges last sample, resets on queue restart
    start_blocked = None
    start_offline = None

    for line in history_lines:
        try:
            tick, online, pulls, queue, dummy, blocked, merged = parse_line(
                *line.strip().split(' '))
        except TypeError:  # line does not fit expected criteria
            continue
        if tick < datetime.datetime.now() - datetime.timedelta(days=DAYS+14):
            continue
        if not pulls and not queue and not merged:  # Bad sample
            continue

        if merged >= last_merge:
            did_merge = merged - last_merge
        elif online:  # Restarts reset the number to 0
            did_merge = merged
        else:
            did_merge = 0

        last_merge = merged
        for moments in happy_moments.values():
            moments.append(int(bool(online and not blocked)))

        for val in real_merges.values():
            val += did_merge
        if queue or did_merge:
            for val in active_merges.values():
                val += did_merge

        if not start_offline and not online:
            start_offline = tick
        if start_offline and online:
            results.offline_intervals.append((start_offline, tick))
            start_offline = None

        if not online:  # Skip offline entries
            continue

        results.append(
            tick, did_merge, pulls, queue, real_merges, active_merges, happy_moments)

        if not start_blocked and blocked:
            start_blocked = tick
        if start_blocked and not blocked:
            results.blocked_intervals.append((start_blocked, tick))
            start_blocked = None
    if tick and not online:
        tick = datetime.datetime.utcnow()
        results.append(
            tick, 0, pulls, queue, real_merges, active_merges, happy_moments)
    if start_blocked:
        results.blocked_intervals.append((start_blocked, tick))
    if start_offline:
        results.offline_intervals.append((start_offline, tick))
    return results


def render_backlog(results, ax_backlog):
    """Render how long items spend in the queue."""
    dts = results.dts
    backlog = results.backlog
    ax_backlog.yaxis.tick_right()
    cur = backlog[1][-1]
    color = backlog_color(cur)
    p_day, = ax_backlog.plot(dts, backlog[1], '%s-' % color)
    p_week, = ax_backlog.plot(dts, backlog[14], 'k:')
    if max(backlog[1]) > 100 or max(backlog[14]) > 100:
        ax_backlog.set_ylim([0, 100])
    ax_backlog.set_ylabel('Backlog')
    ax_backlog.legend(
        [p_day, p_week],
        ['1d avg: %.1f hr wait' % cur, '14d avg: %.1f hr wait' % backlog[14][-1]],
        'lower left',
        fontsize='x-small',
    )

def render_merges(results, ax_merged):
    """Render information about the merge rate."""
    dts = results.dts
    ax_merged.yaxis.tick_right()
    merge_rate = results.merge_rate
    color = merge_color(results.active_merge_rate[1][-1])
    p_day, = ax_merged.plot(dts, merge_rate[1], '%s-' % color)
    p_active, = ax_merged.plot(dts, results.active_merge_rate[1], '%s:' % color)
    p_week, = ax_merged.plot(dts, merge_rate[14], 'k:')
    ax_merged.set_ylabel('Merge rate')
    ax_merged.legend(
        [p_active, p_day, p_week],
        ['active rate: %.1f PRs/day' % results.active_merge_rate[1][-1],
         'real rate: %.1f PRs/day' % merge_rate[1][-1],
         'real 14d avg: %.1f PRs/day' % merge_rate[14][-1]],
        'lower left',
        fontsize='x-small',
    )


def render_health(results, ax_health):
    """Render the percentage of time the queue is healthy/online."""
    # pylint: disable=too-many-locals
    dts = results.dts
    happiness = results.happiness
    ax_health.yaxis.tick_right()

    health_color = '%s-' % happy_color(happiness[1][-1])
    p_1dhealth, = ax_health.plot(dts, happiness[1], health_color)
    p_14dhealth, = ax_health.plot(dts, happiness[14], 'k:')
    cur = 100 * happiness[1][-1]
    cur14 = 100 * happiness[14][-1]
    ax_health.set_ylabel('Unblocked %')

    ax_health.set_ylim([0.0, 1.0])
    ax_health.set_xlim(
        left=datetime.datetime.now() - datetime.timedelta(days=DAYS))

    for start, end in results.blocked_intervals:
        ax_health.axvspan(start, end, alpha=0.2, color='brown', linewidth=0)
    for start, end in results.offline_intervals:
        ax_health.axvspan(start, end, alpha=0.2, color='black', linewidth=0)

    patches = [
        p_1dhealth,
        p_14dhealth,
        mpatches.Patch(color='brown', label='blocked', alpha=0.2),
        mpatches.Patch(color='black', label='offline', alpha=0.2),
    ]

    ax_health.legend(
        patches,
        ['1d avg: %.1f%%' % cur, '14d avg: %.1f%%' % cur14, 'blocked', 'offline'],
        'lower left',
        fontsize='x-small',
    )


def render_queue(results, ax_open):
    """Render the queue graph (open prs, queued, prs)."""
    dts = results.dts
    prs = results.prs
    queued = results.queued
    queue_avg = results.queue_avg
    ax_queued = ax_open.twinx()
    p_open, = ax_open.plot(dts, prs, 'b-')
    color_depth = depth_color(queued[-1])
    p_queue, = ax_queued.plot(dts, queued, color_depth)
    p_14dqueue, = ax_queued.plot(dts, queue_avg, 'k:')
    ax_queued.legend(
        [p_open, p_queue, p_14dqueue],
        [
            'open: %d PRs' % prs[-1],
            'approved: %d PRs' % queued[-1],
            '14d avg: %.1f PRs' % queue_avg[-1],
        ],
        'lower left',
        fontsize='x-small',
    )



def render(results, out_file):
    """Render three graphs to outfile from results."""
    fig, (ax_backlog, ax_merges, ax_open, ax_health) = plt.subplots(
        4, sharex=True, figsize=(16, 10), dpi=100)

    fig.autofmt_xdate()
    plt.gca().xaxis.set_major_formatter(mdates.DateFormatter('%m/%d/%Y'))
    plt.gca().xaxis.set_major_locator(mdates.DayLocator())
    if results.dts:
        render_queue(results, ax_open)
        render_merges(results, ax_merges)
        render_backlog(results, ax_backlog)
        render_health(results, ax_health)
        fig.text(
            0.1, 0.00,
            'image: %s, sample: %s' % (
                datetime.datetime.utcnow().strftime('%Y-%m-%d %H:%M'),
                results.dts[-1].strftime('%Y-%m-%d %H:%M'),
            ),
            horizontalalignment='left',
            fontsize='x-small',
            color=fresh_color(results.dts[-1]),
        )

    plt.savefig(out_file, bbox_inches='tight', format='svg')
    plt.close()


def render_forever(history_uri, img_uri, service_account=None):
    """Download results from history_uri, render to svg and save to img_uri."""
    if service_account:
        print >>sys.stderr, 'Activating service account using: %s' % (
            service_account)
        subprocess.check_call([
            'gcloud',
            'auth',
            'activate-service-account',
            '--key-file=%s' % service_account,
        ])
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
            results = Results()
            output(history.split('\n')[-60*24*DAYS:], results)  # Last 21 days
            render(results, compressed)

        print >>sys.stderr, 'Copy buffer to %s...' % img_uri
        proc = subprocess.Popen(
            ['gsutil', '-q',
             '-h', 'Content-Type:image/svg+xml',
             '-h', 'Cache-Control:public, max-age=%d' % (
                 60 if service_account else 5),
             '-h', 'Content-Encoding:gzip',  # GCS decompresses if necessary
             'cp', '-a', 'public-read', '-', img_uri],
            stdin=subprocess.PIPE)
        proc.communicate(buf.getvalue())
        code = proc.wait()
        if code:
            print >>sys.stderr, 'Failed to copy rendering to %s: %d' % (
                img_uri, code)
        time.sleep(60)



if __name__ == '__main__':
    # log all arguments.
    pprint.PrettyPrinter(stream=sys.stderr).pprint(sys.argv)

    render_forever(*sys.argv[1:])  # pylint: disable=no-value-for-parameter
