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

import re


# Match a specific word
def wordRE(word):
    return re.compile(r'\b(%s)\b' % word, re.IGNORECASE)

# Match lines with error messages
error_re = re.compile(
    r'\b(error|fatal|failed|build timed out)\b', re.IGNORECASE)

# Match the dictionary string in the given line
def objref(line):
    return re.search(r'Event\(api\.ObjectReference(\{.+\})', line)

def combine_wordsRE(words_list):
	return re.compile(r'\b(%s)\b' % '|'.join(words_list), re.IGNORECASE)

log_re = re.compile(r'[^/]+\.log$')
