#!/usr/bin/env python
# -*- coding: utf-8 -*-

# Copyright 2024 The Kubernetes Authors.
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

"""Generate job configuration files."""

import configparser
import glob
import sys

import jinja2


def gen() -> int:
    """Generate job configuration files."""
    for fname in glob.glob("*.ini"):
        config = configparser.ConfigParser()
        config.read(fname)

        name = fname[:-4]
        with open(f"{name}.yaml.jinja", encoding="utf-8") as inp:
            template = jinja2.Template(inp.read())
            beginning = True
            with open(f"{name}.yaml", "w", encoding="utf-8") as out:
                for section in config.sections():
                    out.write(
                        template.render(
                            config[section], job_name=section, beginning=beginning
                        )
                    )
                    beginning = False


if __name__ == "__main__":
    sys.exit(gen())
