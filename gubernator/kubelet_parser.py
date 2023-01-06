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
import json

import jinja2

import regex

def parse(lines, highlight_words, filters, objref_dict):
    """
    Given filters returns indices of wanted lines from log

    Args:
        lines: array of log lines
        highlight_words: array of words that need to be bolded
        filters: dictionary of which filters to apply
        objref_dict: a dictionary where the keys are possible filters
        and the values are the words to be highlighted
    Returns:
        matched_lines: ordered array of indices of lines to display
        highlight_words: updated highlight_words
    """
    matched_lines = []

    if not filters["pod"] and objref_dict:
        highlight_words = []

    # If the filter is on, look for it in the objref_dict
    for k in filters:
        if k != "pod" and filters[k] and k in objref_dict:
            highlight_words.append(objref_dict[k])

    words_re = regex.combine_wordsRE(highlight_words)

    for n, line in enumerate(lines):
        if words_re.search(line):
            matched_lines.append(n)

    return matched_lines, highlight_words


def make_dict(data, pod_re, objref_dict):
    """
    Given the log file and the failed pod name, returns a dictionary
    containing the namespace, UID, and other information associated with the pod
    and a bool indicating if the pod name string is in the log file.

    This dictionary is lifted from the line with the ObjectReference
    """
    pod_in_file = False

    lines = unicode(jinja2.escape(data)).split('\n')
    for line in lines:
        if pod_re.search(line):
            pod_in_file = True
            objref = regex.objref(line)
            containerID = regex.containerID(line)
            if containerID and not objref_dict.get("ContainerID"):
                objref_dict["ContainerID"] = containerID.group(1)
            if objref:
                objref_dict_re = objref.group(1)
                objref_dict_re = re.sub(r'(\w+):', r'"\1": ', objref_dict_re)
                objref_dict_re = objref_dict_re.replace('&#34;', '"')
                objref_dict_re = json.loads(objref_dict_re)
                objref_dict_re.update(objref_dict)
                return objref_dict_re, pod_in_file

    return objref_dict, pod_in_file
