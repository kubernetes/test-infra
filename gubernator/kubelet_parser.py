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
    Given filters returns indeces of wanted lines from log

    Args:
        lines: array of log lines
        hilight_words: array of words that need to be bolded
        filters: dictionary of which filters to apply
        objref_dict: a dictionary where the keys are possible filters 
        and the values are the words to be hilighted 
    Returns:
        matched_lines: ordered array of indeces of lines to display
        hilight_words: updated hilight_words
    """
    matched_lines = []
    
    # If the filter is on, look for it in the objref_dict
    for k in filters:
        if k != "pod" and filters[k] and objref_dict[k]:
            hilight_words.append(objref_dict[k])

    words_re = regex.combine_wordsRE(hilight_words)

    for n, line in enumerate(lines):
        if words_re.search(line):
            matched_lines.append(n)

    return matched_lines, hilight_words


def make_dict(data, pod_re):
    """
    Given the log file and the failed pod name, returns a dictionary
    containing the namespace, UID, and other information associated with the pod.

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
