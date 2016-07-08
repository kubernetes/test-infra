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

import logging
import datetime
import os
import re

import jinja2

def parse(lines, error_re, hilight_res, filters):
    matched_lines = []
    UID = ""

    end = 0
    event_re = re.compile(r'.*Event\(api\.ObjectReference.*UID:&#34;(.*)&#34;, A.*')
    for n, line in enumerate(lines):
        if error_re.search(line):
            matched_lines.append(n)
            if filters["uid"] and UID == "":
                s = event_re.search(line)
                if s and s.group(1) != "":
                    end = n
                    UID = s.group(1)
                    regex = r'\b(' + UID + r')\b'
                    uid_re = re.compile(regex, re.IGNORECASE)
                    hilight_res.append(uid_re)

        if UID != "" and matched_lines[-1] != n:
            if uid_re.search(line):
                matched_lines.append(n)
        matched_lines.sort()

    return matched_lines, hilight_res
