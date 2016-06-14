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
import datetime

dts = []
prs = []
queued = []
instant_happiness = []
daily_happiness = []

blocked_intervals = []
daily_queue = collections.deque()

with open('results.txt', 'r') as f:
    last_blocked = None
    daily_sum = 0
    for _ in xrange(60 * 24 * 21):
        f.readline()
    for line in f:
        date, time, online, pr, queue, run, blocked = line.strip().split(' ')
        pr, queue, run = int(pr), int(queue), int(run)
        dt = datetime.datetime.strptime('{} {}'.format(date, time), '%Y-%m-%d %H:%M:%S.%f')
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

import matplotlib
matplotlib.use('Agg')
import matplotlib.pyplot as plt
import matplotlib.dates as mdates

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
    fig.text(.1, .08, "Health for the last week: %.0f%%" % (100.0 * week_happy / week_points))

plt.savefig('k8s-queue-health.png', bbox_inches='tight')
# plt.show()
