# Copyright 2025 The Kubernetes Authors.
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

import argparse
import configparser
import filecmp
import os
import pathlib
import shutil
import sys
import tempfile
import typing

import jinja2


def generate(patterns: typing.List[str], verify: bool) -> typing.List[str]:
    """
    Generate job configuration files.
    Return list of errors.
    """
    errors = []
    for pattern in patterns:
        paths = pathlib.Path(".").glob(pattern)
        if not paths:
            errors.append(f"No files found for pattern {pattern}")
            continue
        for path in paths:
            errs = generate_one(path, verify)
            if errs:
                errors.extend(errs)
    return errors


def generate_one(path: pathlib.Path, verify: bool) -> typing.List[str]:
    """
    Generate job configuration files from one template.
    Return list of errors.
    """
    config = configparser.ConfigParser(inline_comment_prefixes=["#"])
    config.read_file(path.open())

    template_name = config.get("DEFAULT", "template")
    template_path = path.parent / template_name
    errors = []
    with template_path.open() as inp:
        template = jinja2.Template(inp.read(), lstrip_blocks=True)
        jobs = dict(
            item.split(":") for item in config.get("DEFAULT", "files").split(",")
        )
        for name, job in jobs.items():
            tmp = tempfile.NamedTemporaryFile(
                "w",
                prefix=f"{template_name}.",
                delete=False,
            )
            with tmp:
                header = (
                    "# GENERATED FILE - DO NOT EDIT!\n#\n# "
                    f"Instead, modify {template_name} and run `make generate-jobs`.\n"
                )
                for section in config.sections():
                    # generate job types if they're mentioned in
                    # the `generate` section key or if the key is not set
                    gen = config[section].get("generate")
                    if gen is not None:
                        gen_jobs = gen.split(",")
                        # validate job types mentioned in the `generate` section key
                        for gen_job in gen_jobs:
                            if gen_job and gen_job not in jobs:
                                errors.append(
                                    f"Can't generate {name} job {section}: "
                                    f"unknown job type: {gen_job}"
                                )
                        # skip job types not mentioned in the `generate` section key
                        if name not in gen_jobs:
                            continue

                    args = dict(config[section])
                    args[name] = True
                    tmp.write(
                        template.render(
                            args,
                            job_name=job.format(section=section),
                            header=header,
                        )
                    )
                    header = ""
                tmp.write("\n")
            out = template_path.parent / f"{template_path.stem}-{name}.yaml"
            if not os.path.exists(out):
                if verify:
                    os.unlink(tmp.name)
                    errors.append(f"Can't verify content: {out} doesn't exist")
                    continue
            else:
                equal = filecmp.cmp(out, tmp.name, shallow=False)
                if verify:
                    os.unlink(tmp.name)
                    if not equal:
                        errors.append(
                            f"Generated content for {out} differs from existing"
                        )
                    continue
                if equal:
                    os.unlink(tmp.name)
                    continue
            shutil.move(tmp.name, out)

    return errors


def main(argv):
    """Entry point."""
    parser = argparse.ArgumentParser(
        prog="Jobs Generator",
        description="Generate job configuration files from templates",
        formatter_class=argparse.RawTextHelpFormatter,
    )
    parser.add_argument(
        "pattern",
        nargs="+",
        help="config path pattern in the Python pathlib pattern language format:\n"
        "https://docs.python.org/3/library/pathlib.html#pattern-language,\n"
        "for example: config/jobs/**/*.generate.conf",
    )
    parser.add_argument(
        "--verify",
        action="store_true",
        help="Verify if generated files are the same as existing",
    )
    args = parser.parse_args(argv)

    errors = generate(args.pattern, args.verify)
    if errors:
        for err in errors:
            print(err, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
