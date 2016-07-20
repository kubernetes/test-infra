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

            # If the line is the ObjectReference line, make a dictionary
            objref = regex.objref(line)
            if objref and objref.group(1) != "":
                objref_dict = objref.group(1)        
                keys = regex.keys_re.findall(objref_dict)
                
                for k in keys:
                    objref_dict = regex.key_to_string(k, objref_dict)

                # Convert string into dictionary
                objref_dict = ast.literal_eval(regex.fix_quotes(objref_dict))

                if uid == "" and filters["uid"] and objref_dict["UID"]:
                    uid = objref_dict["UID"]
                    hilight_words.append(uid)
                if namespace == "" and filters["namespace"] and objref_dict["Namespace"]:
                    namespace = objref_dict["Namespace"]
                    hilight_words.append(namespace)

        if uid != "" and matched_lines[-1] != n:
            uid_re = regex.wordRE(uid)
            if uid_re.search(line):
                matched_lines.append(n)
        matched_lines.sort()

    return matched_lines, hilight_words
