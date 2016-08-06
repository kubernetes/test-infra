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

import jinja2

import kubelet_parser
import regex

CONTEXT = 4


def hilight(line, hilight_words):
    # Join all the words that need to be bolded into one regex
    words_re = regex.combine_wordsRE(hilight_words)
    line = words_re.sub(r'<span class="keyword">\1</span>', line)
    return '<span class="hilight">%s</span>' % line


def log_html(lines, matched_lines, hilight_words, skip_fmt):
    """
    Constructs the html for the filtered log
    Given:
        lines: list of all lines in the log
        matched_lines: list of lines that have a filtered string in them
        hilight_words: list of words to be bolded
        skip_fmt: function producing string to replace the skipped lines
    Returns:
        output: list of a lines HTML code suitable for inclusion in a <pre>
        tag, with "interesting" errors hilighted
    """
    output = []

    matched_lines.append(len(lines))  # sentinel value

    last_match = None
    for match in matched_lines:
        if last_match is not None:
            previous_end = min(match, last_match + CONTEXT + 1)
            output.extend(lines[last_match + 1: previous_end])
        else:
            previous_end = 0
        skip_amount = match - previous_end - CONTEXT
        if skip_amount > 100:
            output.append('<span class="skip">%s</span>' % skip_fmt(skip_amount))
        elif skip_amount > 1:
            skip_id = 'skip_%s' % match
            skip_string = ('<span class="skip">'
                '<a href="javascript:show_skipped(\'%s\')" onclick="this.style.display=\'none\'">'
                '%s</a></span>'
                '<span class="skipped" id=%s style="display:none; float: left;">') % (skip_id,
                skip_fmt(skip_amount), skip_id)
            lines[previous_end] = "%s%s" % (skip_string, lines[previous_end])
            output.extend(lines[previous_end:match-CONTEXT])
            output.append('</span>')
        elif skip_amount == 1:  # pointless say we skipped 1 line
            output.append(lines[previous_end])
        if match == len(lines):
            break
        output.extend(lines[max(previous_end, match - CONTEXT): match])
        output.append(hilight(lines[match], hilight_words))
        last_match = match

    return output

def digest(data, skip_fmt=lambda l: '... skipping %d lines ...' % l,
      objref_dict=None, filters=None, error_re=regex.error_re):
    """
    Given a build log, return a chunk of HTML code suitable for
    inclusion in a <pre> tag, with "interesting" errors hilighted.

    This is similar to the output of `grep -C4` with an appropriate regex.
    """
    lines = unicode(jinja2.escape(data)).split('\n')

    if filters is None:
        filters = {'Namespace': '', 'UID': '', 'pod': '', 'ContainerID':''}

    hilight_words = ["error", "fatal", "failed", "build timed out"]
    if filters["pod"]:
        hilight_words = [filters["pod"]]

    if not (filters["UID"] or filters["Namespace"] or filters["ContainerID"]):
        matched_lines = [n for n, line in enumerate(lines) if error_re.search(line)]
    else:
        matched_lines, hilight_words = kubelet_parser.parse(lines,
            hilight_words, filters, objref_dict)

    output = log_html(lines, matched_lines, hilight_words, skip_fmt)

    return '\n'.join(output)


if __name__ == '__main__':
    import sys
    for f in sys.argv[1:]:
        print digest(open(f).read().decode('utf8'))
