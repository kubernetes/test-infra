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
import ast

import jinja2

import regex

def parse(lines, error_re, hilight_words, filters):
    """
    Given filters returns indeces of wanted lines from the kubelet log

    Args:
        lines: array of kubelet log lines
        error_re: regular expression of the failed pod name
        hilight_words: array of words that need to be bolded
        filters: dictionary of which filters to apply
    Returns:
        matched_lines: ordered array of indeces of lines to display
        hilight_words: updated hilight_words
    """
    matched_lines = []
    uid = ""
    namespace = ""

    for n, line in enumerate(lines):
        if error_re.search(line):
            matched_lines.append(n)



    return matched_lines, hilight_words
