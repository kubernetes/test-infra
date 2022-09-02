#!/usr/bin/env python3

# Copyright 2020 The Kubernetes Authors.
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

"""Recursive YAML comparator

This script allow the user to compare two directories containing
YAML files in the same folder structure.

Usage: ./compare-yaml.py path1 path2

It will walk through all files in path1 and compare them with the same file
found in path2. Comparison is semantic and not affected by the order of lists.
"""

import operator
import os
import sys

import yaml


def loadfile(f):
    """Loads a YAML file containing several documents into a list of objects

    :param f: string containing the path to the file to load
    :return: list of loaded objects
    """
    try:
        return list(yaml.safe_load_all(open(f)))
    except Exception as e:
        print('Exception caught loading', f, e)
        raise


def compare(f1, left, right):
    """Compares the same filename from two path roots and prints the difference to stdout

    :param f1: full path to the first file
    :param left: root of f1 path to construct relative path
    :param right: second root to deduct f2 path
    :return: None
    """
    f2 = os.path.join(right, os.path.relpath(f1, left))
    if not os.path.isfile(f2):
        print('Cannot find', f2)
        return
    d1s = loadfile(f1)
    d2s = loadfile(f2)
    if not all(map(operator.eq, d1s, d2s)):
        print('meld', f1, f2)


def main():
    if len(sys.argv) != 3:
        sys.exit('Provide path1 and path2')
    left = sys.argv[1]
    right = sys.argv[2]
    for root, _, files in os.walk(left):
        for name in files:
            f = os.path.join(root, name)
            if not os.path.isfile(f):
                print('Skipping', f)
                continue
            if f.endswith('.yaml'):
                compare(f, left, right)


if __name__ == "__main__":
    main()
