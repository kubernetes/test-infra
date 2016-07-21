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

def parse(lines, hilight_words, filters, objref_dict):
    """
    Given filters returns indeces of wanted lines from the kubelet log

    Args:
        lines: array of kubelet log lines
        hilight_words: array of words that need to be bolded
        filters: dictionary of which filters to apply
    Returns:
        matched_lines: ordered array of indeces of lines to display
        hilight_words: updated hilight_words
    """
    matched_lines = []
    
    if filters["uid"] and objref_dict["UID"]:
        uid = objref_dict["UID"]
        hilight_words.append(uid)
    if filters["namespace"] and objref_dict["Namespace"]:
        namespace = objref_dict["Namespace"]
        hilight_words.append(namespace)

    words_re = regex.combine_wordsRE(hilight_words)

    for n, line in enumerate(lines):
        if words_re.search(line):
            matched_lines.append(n)

    return matched_lines, hilight_words


def make_dict(data, pod_re):
    """
    Given the kubelet log file and the failed pod name, returns a dictionary
    containing the namespace and UID associated with the pod.

    This dictionary is lifted from the line with the ObjectReference
    """
    lines = unicode(jinja2.escape(data)).split('\n')
    for line in lines:
        if pod_re.search(line):
            objref = regex.objref(line)
            if objref and objref.group(1) != "":
                objref_dict = objref.group(1)        
                keys = regex.keys_re.findall(objref_dict)
                
                for k in keys:
                    objref_dict = regex.key_to_string(k, objref_dict)

                # Convert string into dictionary
                objref_dict = ast.literal_eval(regex.fix_quotes(objref_dict))
                return objref_dict
