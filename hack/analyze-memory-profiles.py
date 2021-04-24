#!/usr/bin/env python3

# Copyright 2021 The Kubernetes Authors.
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

# This script is meant to be used to analyize memory profiles created by the sidecar utility when
# the sidecar.Options.WriteMemoryProfile option has been set. The tool will write sequential
# profiles into a directory, from which this script can load the data, create time series and
# visualize them.

import os
import subprocess
import sys
from datetime import datetime

from matplotlib.font_manager import FontProperties
import matplotlib.dates as mdates
import matplotlib.pyplot as plt
import matplotlib.ticker as ticker

if len(sys.argv) != 2:
    print("[ERROR] Expected the directory containing profiles as the only argument.")
    print("Usage: {} ./path/to/profiles/".format(sys.argv[0]))
    sys.exit(1)

profile_dir = sys.argv[1]

def parse_bytes(value):
    # we will either see a raw number or one with a suffix
    value = value.decode("utf-8")
    if not value.endswith("B"):
        return float(value)

    suffix = value[-2:]
    multiple = 1
    if suffix == "KB":
        multiple = 1024
    elif suffix == "MB":
        multiple = 1024 * 1024
    elif suffix == "GB":
        multiple = 1024 * 1024 * 1024

    return float(value[:-2]) * multiple


dates_by_name = {}
flat_usage_over_time = {}
cumulative_usage_over_time = {}
max_usage = 0

for subdir, dirs, files in os.walk(profile_dir):
    for file in files:
        output = subprocess.run(
            ["go", "tool", "pprof", "-top", "-inuse_space", os.path.join(subdir, file)],
            check=True, stdout=subprocess.PIPE
        )
        # The output of go tool pprof will look like:
        #
        # File: sidecar
        # Type: inuse_space
        # Time: Mar 19, 2021 at 10:30am (PDT)
        # Showing nodes accounting for 66.05MB, 100% of 66.05MB total
        #       flat  flat%   sum%        cum   cum%
        #       64MB 96.90% 96.90%       64MB 96.90%  google.golang.org/api/internal/gensupport...
        #
        # We want to parse all of the lines after the header and metadata.
        lines = output.stdout.splitlines()
        date = datetime.strptime(
            lines[2].decode("utf-8").replace("am", "AM").replace("pm", "PM"),
            "Time: %b %d, %Y at %H:%M%p (%Z)"
        )
        usage = parse_bytes(lines[3].split()[-2])
        if usage > max_usage:
            max_usage = usage
        data_index = 0
        for i in range(len(lines)):
            if lines[i].split()[0].decode("utf-8") == "flat":
                data_index = i + 1
                break
        for line in lines[data_index:]:
            parts = line.split()
            name = parts[5]
            if name not in dates_by_name:
                dates_by_name[name] = []
            dates_by_name[name].append(date)
            if name not in flat_usage_over_time:
                flat_usage_over_time[name] = []
            flat_usage_over_time[name].append(parse_bytes(parts[0]))
            if name not in cumulative_usage_over_time:
                cumulative_usage_over_time[name] = []
            cumulative_usage_over_time[name].append(parse_bytes(parts[3]))

plt.rcParams.update({'font.size': 22})
fig = plt.figure(figsize=(30, 18))
plt.subplots_adjust(right=0.7)
ax = plt.subplot(211)
for name in dates_by_name:
    dates = mdates.date2num(dates_by_name[name])
    values = flat_usage_over_time[name]
    # we only want to show the top couple callsites, or our legend gets noisy
    if max(values) > 0.01 * max_usage:
        ax.plot_date(dates, values, label=name.decode("utf-8"), linestyle='solid')
    else:
        ax.plot_date(dates, values, linestyle='solid')
ax.set_yscale('log')
formatter = ticker.FuncFormatter(lambda y, pos: '{:,.0f}'.format(y / (1024 * 1024)) + 'MB')
ax.yaxis.set_major_formatter(formatter)
plt.xlabel("Time")
plt.ylabel("Flat Space In Use (bytes)")
plt.title("Space In Use By Callsite")
fontP = FontProperties()
fontP.set_size('xx-small')
plt.legend(bbox_to_anchor=(1, 1), loc='upper left', prop=fontP)

ax = plt.subplot(212)
for name in dates_by_name:
    dates = mdates.date2num(dates_by_name[name])
    values = cumulative_usage_over_time[name]
    # we only want to show the top couple callsites, or our legend gets noisy
    if max(values) > 0.01 * max_usage:
        ax.plot_date(dates, values, label=name.decode("utf-8"), linestyle='solid')
    else:
        ax.plot_date(dates, values, linestyle='solid')
ax.set_yscale('log')
ax.yaxis.set_major_formatter(formatter)
plt.xlabel("Time")
plt.ylabel("Cumulative Space In Use (bytes)")
fontP = FontProperties()
fontP.set_size('xx-small')
plt.legend(bbox_to_anchor=(1, 1), loc='upper left', prop=fontP)

plt.show()
