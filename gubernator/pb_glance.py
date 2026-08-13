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

"""
A tiny, minimal protobuf2 parser that's able to extract enough information
to be useful.
"""

import cStringIO as StringIO


def parse_protobuf(data, schema=None):
    """
    Do a simple parse of a protobuf2 given minimal type information.

    Args:
        data: a string containing the encoded protocol buffer.
        schema: a dict containing information about each field number.
            The keys are field numbers, and the values represent:
                - str: the name of the field
                - dict: schema to recursively decode an embedded message.
                        May contain a 'name' key to name the field.
    Returns:
        dict: mapping from fields to values. The fields may be strings instead of
            numbers if schema named them, and the value will *always* be
            a list of values observed for that key.
    """
    if schema is None:
        schema = {}

    buf = StringIO.StringIO(data)

    def read_varint():
        out = 0
        shift = 0
        c = 0x80
        while c & 0x80:
            c = ord(buf.read(1))
            out = out | ((c & 0x7f) << shift)
            shift += 7
        return out

    values = {}

    while buf.tell() < len(data):
        key = read_varint()
        wire_type = key & 0b111
        field_number = key >> 3
        field_name = field_number
        if wire_type == 0:
            value = read_varint()
        elif wire_type == 1:  # 64-bit
            value = buf.read(8)
        elif wire_type == 2:  # length-delim
            length = read_varint()
            value = buf.read(length)
            if isinstance(schema.get(field_number), basestring):
                field_name = schema[field_number]
            elif field_number in schema:
                # yes, I'm using dynamic features of a dynamic language.
                # pylint: disable=redefined-variable-type
                value = parse_protobuf(value, schema[field_number])
                field_name = schema[field_number].get('name', field_name)
        elif wire_type == 5:  # 32-bit
            value = buf.read(4)
        else:
            raise ValueError('unhandled wire type %d' % wire_type)
        values.setdefault(field_name, []).append(value)

    return values
