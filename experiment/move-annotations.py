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

"""ProwJob annotations relocator

This script allows the user to alter ProwJobs syntax by relocating
names and annotations to the top of the YAML elements for clarity.

Only those two fields are relocated to improve diff readability and
the document itself should be unchanged semantically (you can use the
compare-yaml.py script to ensure that).

Usage: ./move-annotations.py path

It will walk through all files with .yaml extension found under path
and modify them in-place.
"""

import os
import sys

import ruamel.yaml

yaml = ruamel.yaml.YAML()
yaml.preserve_quotes = True


def move_annotations(f):
    """Modifies a YAML ProwJob file in-place by moving name and annotations
    to the top of the spec elements.

    :param f:
    :return:
    """
    files = list(yaml.load_all(open(f)))
    # pylint: disable=R1702
    for lvl1 in files:
        for lvl2 in lvl1.values():
            if isinstance(lvl2, ruamel.yaml.comments.CommentedSeq):
                for job in lvl2:
                    if not 'annotations' in job:
                        continue
                    job.move_to_end('annotations', last=False)
                    job.move_to_end('name', last=False)
            elif isinstance(lvl2, ruamel.yaml.comments.CommentedMap):
                for lvl3 in lvl2.values():
                    if isinstance(lvl3, bool):
                        continue
                    for job in lvl3:
                        if not 'annotations' in job:
                            continue
                        job.move_to_end('annotations', last=False)
                        job.move_to_end('name', last=False)
            else:
                print('skipping', lvl2)
    yaml.dump_all(files, open(f, 'w'))


def main():
    if len(sys.argv) != 2:
        sys.exit('Provide path to jobs')
    for root, _, files in os.walk(sys.argv[1]):
        for name in files:
            f = os.path.join(root, name)
            if not os.path.isfile(f):
                print('Skipping non file', f)
                continue
            if f.endswith('.yaml'):
                try:
                    move_annotations(f)
                except Exception as e:
                    print('Caught exception processing', f, e)
                    raise


if __name__ == "__main__":
    main()
