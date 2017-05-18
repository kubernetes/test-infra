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

import logging

import jinja2

import kubelet_parser
import regex

CONTEXT_DEFAULT = 6
MAX_BUFFER = 5000000  # GAE has RAM limits.


def highlight(line, highlight_words):
    # Join all the words that need to be bolded into one regex
    words_re = regex.combine_wordsRE(highlight_words)
    line = words_re.sub(r'<span class="keyword">\1</span>', line)
    return '<span class="highlight">%s</span>' % line


def log_html(lines, matched_lines, highlight_words, skip_fmt):
    """
    Constructs the html for the filtered log
    Given:
        lines: list of all lines in the log
        matched_lines: list of lines that have a filtered string in them
        highlight_words: list of words to be bolded
        skip_fmt: function producing string to replace the skipped lines
    Returns:
        output: list of a lines HTML code suitable for inclusion in a <pre>
        tag, with "interesting" errors highlighted
    """
    output = []

    matched_lines.append(len(lines))  # sentinel value

    # Escape hatch: if we're going to generate a LOT of output, try to trim it down.
    context_lines = CONTEXT_DEFAULT
    if len(matched_lines) > 2000:
        context_lines = 0

    last_match = None
    for match in matched_lines:
        if last_match is not None:
            previous_end = min(match, last_match + context_lines + 1)
            output.extend(lines[last_match + 1: previous_end])
        else:
            previous_end = 0
        if match == len(lines):
            context_lines = 0
        skip_amount = match - previous_end - context_lines
        if skip_amount > 1:
            output.append('<span class="skip" data-range="%d-%d">%s</span>' %
                          (previous_end, match - context_lines, skip_fmt(skip_amount)))
        elif skip_amount == 1:  # pointless say we skipped 1 line
            output.append(lines[previous_end])
        if match == len(lines):
            break
        output.extend(lines[max(previous_end, match - context_lines): match])
        output.append(highlight(lines[match], highlight_words))
        last_match = match

    return output


def truncate(data, limit=MAX_BUFFER):
    if len(data) <= limit:
        return data

    # If we try to process more than MAX_BUFFER, things will probably blow up.
    half = limit / 2
    # Erase the intermediate lines, but keep the line count consistent so
    # skip line expansion works.
    cut_newlines = data[half:-half].count('\n')

    logging.warning('truncating buffer %.1f times too large (%d lines erased)',
                    len(data) / float(limit), cut_newlines)

    return ''.join([data[:half], '\n' * cut_newlines, data[-half:]])

def digest(data, objref_dict=None, filters=None, error_re=regex.error_re,
    skip_fmt=lambda l: '... skipping %d lines ...' % l):
    # pylint: disable=too-many-arguments
    """
    Given a build log, return a chunk of HTML code suitable for
    inclusion in a <pre> tag, with "interesting" errors highlighted.

    This is similar to the output of `grep -C4` with an appropriate regex.
    """
    if isinstance(data, str):  # the test mocks return str instead of unicode
        data = data.decode('utf8', 'replace')
    lines = unicode(jinja2.escape(truncate(data))).split('\n')

    if filters is None:
        filters = {'Namespace': '', 'UID': '', 'pod': '', 'ContainerID':''}

    highlight_words = regex.default_words

    if filters["pod"]:
        highlight_words = [filters["pod"]]

    if not (filters["UID"] or filters["Namespace"] or filters["ContainerID"]):
        matched_lines = [n for n, line in enumerate(lines) if error_re.search(line)]
    else:
        matched_lines, highlight_words = kubelet_parser.parse(lines,
            highlight_words, filters, objref_dict)

    output = log_html(lines, matched_lines, highlight_words, skip_fmt)
    output.append('')

    return '\n'.join(output)


if __name__ == '__main__':
    import sys
    for f in sys.argv[1:]:
        print digest(open(f).read().decode('utf8'))
