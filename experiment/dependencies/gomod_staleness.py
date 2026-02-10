#!/usr/bin/env python3

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

# pylint: disable=line-too-long

import json
import subprocess
import logging
import os
import argparse


def get_dependencies():
    dependencies = {}
    f = subprocess.Popen('go mod edit -json', stdout=subprocess.PIPE, stderr=subprocess.PIPE, shell=True)
    (stdout, stderr) = f.communicate()
    if stderr:
        logging.fatal(stderr)
    data = json.loads(stdout)
    for dep in data["Require"]:
        if not dep['Path'].startswith("k8s.io/"):
            dependencies[dep['Path']] = dep['Version']
    return dependencies


def get_dependencies_to_update(skip_packages=None):
    if skip_packages is None:
        skip_packages = []

    dependencies = []
    f = subprocess.Popen('go mod edit -json | jq -r ".Require[] | .Path | select(contains(\\"k8s.io/\\") | not)"',
                         stdout=subprocess.PIPE, stderr=subprocess.PIPE, shell=True)
    (stdout, stderr) = f.communicate()
    if stderr:
        logging.fatal(stderr)
    for line in stdout.splitlines():
        dep = line.decode('utf-8')
        if dep not in skip_packages:
            dependencies.append(dep)
    return sorted(dependencies)


def sanitize(param):
    # v0.0.0-20190331200053-3d26580ed485
    if param.count('-') == 2:
        return param.split('-')[-1]
    # v11.1.2+incompatible
    if param.count('+') == 1:
        return param.split('+')[0]
    return param


def source(pkg, old, new):
    # pylint: disable=too-many-return-statements, too-many-branches, no-else-return
    if pkg.startswith("cloud.google.com/go"):
        return "github.com/googleapis/google-cloud-go", old, new
    elif pkg.startswith("golang.org/x/"):
        return "github.com/golang" + pkg[len("golang.org/x"):], old, new
    elif pkg.startswith("go.uber.org/"):
        return "github.com/uber-go" + pkg[len("go.uber.org"):], old, new
    elif pkg.startswith("sigs.k8s.io/"):
        return "github.com/kubernetes-sigs" + pkg[len("sigs.k8s.io"):], old, new
    elif pkg.startswith("cel.dev/"):
        return "github.com/google/cel-spec", old, new
    elif pkg.startswith("go.starlark.net"):
        return "github.com/google/starlark-go", old, new
    elif pkg.startswith("gopkg.in"):
        repo = pkg.split('/', 1)[1]  # get the last part of `gopkg.in/gcfg.v1` which is `gcfg.v1`
        repo = repo.split('.', 1)[0]  # get the part before the `.` which is `gcfg`
        # we could end up with `gcfg` or `square/go-jose` here, deal with them separately
        if '/' not in repo:
            repo = "github.com/go-" + repo + "/" + repo
        else:
            repo = "github.com/" + repo
        return repo, old, new
    elif pkg.startswith("google.golang.org"):
        array = pkg.split('/', 2)
        if array[1] == "api":
            repo = "github.com/googleapis/google-api-go-client"
        elif array[1] == "genproto":
            repo = "github.com/googleapis/go-genproto"
        else:  # repo = "protobuf"
            repo = "github.com/protocolbuffers/protobuf-go"
        return repo, old, new
    elif pkg.startswith("go.opentelemetry.io/"):
        array = pkg.split('/', 2)
        if array[1] == "contrib":
            repo = "github.com/open-telemetry/opentelemetry-go-contrib"
        elif array[1] == "proto":
            repo = "github.com/open-telemetry/opentelemetry-proto-go"
        else:  # repo = "otel"
            repo = "github.com/open-telemetry/opentelemetry-go"
        if len(array) > 2:
            path = array[2] + "/"
        else:
            path = ""
        return repo, path + old, path + new
    return pkg, old, new


def parse_arguments():
    parser = argparse.ArgumentParser(description='Update Go module dependencies and report on changes.')
    parser.add_argument('--skip', nargs='*', default=["github.com/libopenstorage/openstorage"],
                        help='List of packages to skip updating (default: github.com/libopenstorage/openstorage)')
    parser.add_argument('--patch-output', type=str,
                        help='Path to save the output from git patch for go.mod/sum')
    parser.add_argument('--markdown-output', type=str,
                        help='Path to save the markdown table of dependency differences')
    return parser.parse_args()


def print_markdown_diff(before, after):
    markdown_lines = []
    markdown_lines.append("Package      | Current       | Latest        | URL")
    markdown_lines.append("------------ | ------------- | ------------- |------------- ")

    for pkg in sorted(before.keys()):
        if pkg.startswith("k8s.io"):
            continue
        old = sanitize(before[pkg])
        if pkg not in after:
            line = "~%s~ | %s | Dropped | NONE" % (pkg, old)
            markdown_lines.append(line)
            print(line)
            continue
        new = sanitize(after[pkg])
        repo, oldtag, newtag = source(pkg, old, new)
        if old != new:
            line = "%s | %s | %s | https://%s/compare/%s...%s " % (pkg, old, new, repo, oldtag, newtag)
            markdown_lines.append(line)
            print(line)
        else:
            line = "~%s~ | %s | %s | No changes" % (pkg, old, new)
            markdown_lines.append(line)
            print(line)

    return markdown_lines


def main():
    args = parse_arguments()

    cleanup_command = """git reset --hard HEAD"""
    print(">>>> Running command %r" % cleanup_command)
    os.system(cleanup_command)

    print(">>>> parsing go.mod before updates")
    before = get_dependencies()
    print("Found %d packages before updating dependencies" % len(before.values()))

    deps_to_update = get_dependencies_to_update(args.skip)
    print("Found %d packages to be updated" % len(deps_to_update))
    print(">>>> Packages that will be updated %r" % deps_to_update)
    print(">>>> Packages that will be skipped %r" % args.skip)

    for pkg in deps_to_update:
        update_command = """\
go get -u %s""" % pkg
        print(">>>> Running command %r" % update_command)
        os.system(update_command)

    print(">>>> Ensuring Packages that will be skipped are at their previous versions %r" % (args.skip,))
    for pkg in args.skip:
        if pkg in before.keys():
            update_command = """\
    go get %s@%s""" % (pkg, before[pkg])
            print(">>>> Running command %r" % update_command)
            os.system(update_command)

    print(">>>> parsing go.mod after updates")
    after = get_dependencies()
    print("Found %d packages after updating dependencies" % len(before.values()))

    print(">>>> Packages that got dropped %r" % (list(set(before.keys()) - set(after.keys()))))
    print(">>>> Packages that were added  %r" % (list(set(after.keys()) - set(before.keys()))))

    os.system("git add . && git commit -m 'gomod_staleness.py: update go.mod'")
    print(">>>>> Patch of differences <<<<")
    patch_output = subprocess.check_output("git --no-pager format-patch -1 HEAD --stdout", shell=True).decode('utf-8')
    print(patch_output)
    if args.patch_output:
        with open(args.patch_output, 'w') as f:
            f.write(patch_output)
        print(f"Patch output saved to {args.patch_output}")

    print(">>>>> Mark down of differences <<<<")
    markdown_lines = print_markdown_diff(before, after)
    if args.markdown_output:
        with open(args.markdown_output, 'w') as f:
            f.write('\n'.join(markdown_lines))
        print(f"Markdown output saved to {args.markdown_output}")


if __name__ == "__main__":
    main()
