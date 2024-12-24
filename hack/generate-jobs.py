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

import argparse
import configparser
import filecmp
import glob
import os
import shutil
import sys
import tempfile
import typing

import jinja2  # pylint: disable=import-error


def generate(patterns: typing.List[str], only_verify: bool, overwrite: bool) -> str:
    """
    Generate job configuration files.
    Return empty string if successful, error message otherwise.
    """
    for pattern in patterns:
        paths = glob.glob(pattern)
        if not paths:
            return f"No files found for pattern {pattern}"
        for fname in paths:
            err = generate_one(fname, only_verify, overwrite)
            if err:
                return err
    return ""


def generate_one(fname: str, only_verify: bool, overwrite: bool) -> str:
    config = configparser.ConfigParser()
    config.read(fname)

    template_name = config.get("DEFAULT", "template")
    template_path = os.path.join(os.path.dirname(fname), template_name)
    name = template_path.split(".")[0]
    with open(template_path, encoding="utf-8") as inp:
        template = jinja2.Template(inp.read(), lstrip_blocks=True)
        for kind in config.get("DEFAULT", "kinds").split(","):
            tmp = tempfile.NamedTemporaryFile(
                "w",
                prefix=os.path.splitext(os.path.basename(__file__))[0],
                delete=False,
            )
            with tmp:
                for section in config.sections():
                    tmp.write(
                        template.render(
                            config[section], kind=kind, job_name=f"{kind}-{section}"
                        )
                    )

            out = f"{name}-{kind}.yaml"
            equal = filecmp.cmp(out, tmp.name, shallow=False)
            if only_verify:
                os.unlink(tmp.name)
                if not equal:
                    return f"Generated content for {out} differs from existing"
                continue
            if equal:
                os.unlink(tmp.name)
                continue
            if os.path.exists(out) and not overwrite:
                os.unlink(tmp.name)
                return (
                    f"Generated content for {out} differs from existing, "
                    "use --owerwrite to update"
                )
            shutil.move(tmp.name, out)
    return ""


def main(argv):
    """Entry point."""
    parser = argparse.ArgumentParser(
        prog="Jobs Generator",
        description="Generate job configuration files from templates",
    )
    parser.add_argument(
        "pattern",
        nargs="+",
        help="config path pattern, e.g config/jobs/kubernetes/sig-node/*.conf",
    )
    parser.add_argument(
        "--only-verify",
        action="store_true",
        help="Only verify if generated files are the same as existing",
    )
    parser.add_argument(
        "--overwrite", action="store_true", help="Owerwrite output files"
    )
    args = parser.parse_args(argv)

    err = generate(args.pattern, args.only_verify, args.overwrite)
    if err:
        print(err, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
