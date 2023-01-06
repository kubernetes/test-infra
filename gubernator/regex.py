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

import re


# Match a specific word
def wordRE(word):
    return re.compile(r'\b(%s)\b' % word, re.IGNORECASE)

# Match lines with error messages
# HACK: match ANSI colored lines by allowing preceding "m",
# as in"\x1b[0;31mFAILED\x1b[0m"

default_words = [
    "build timed out",
    "error",
    "fail",
    "failed",
    "fatal",
    "undefined",
    "panic:",
]

error_re = re.compile(
    r'(?:\b|(?<=m))(%s)\b' % '|'.join(default_words), re.IGNORECASE)

# Match the dictionary string in the given line
def objref(line):
    return re.search(r'api\.ObjectReference(\{.*?&#34;\})', line)

# Combine a list of words into one regex that match any of them
def combine_wordsRE(words_list):
    return re.compile(r'\b(%s)\b' % '|'.join(words_list), re.IGNORECASE)

# Match the file name of a log given a filepath to the log
log_re = re.compile(r'[^/]+\.log$')

# Match the container id given a line containing the pod name
def containerID(line):
    return re.search(r'ContainerID:([0-9A-Fa-f]*)', line)

def timestamp(line):
    return re.search(r'(\d\d-?\d\d[T\s]\d\d:\d\d:\d\d\.\d+)', line)

def sub_timestamp(line):
    return re.sub(r'(-|T|\s)', "", timestamp(line).group(0))
