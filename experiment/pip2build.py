#!/usr/bin/env python2

# Copyright 2017 The Kubernetes Authors.
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

"""Convert a pip install command into BUILD rules.

Usage:
  python pip2build.py <pip-package ...>
"""

from __future__ import absolute_import
from __future__ import division
from __future__ import print_function

import hashlib
import json
import os
import re
import subprocess
import sys
import tarfile
import tempfile
import urllib3

import requests

def stderr(fmt, *a):
    print(fmt % a, file=sys.stderr)


def main(*pkgs):
    if not pkgs:
        return
    urllib3.logging.captureWarnings(True)
    tmp = tempfile.mkdtemp(prefix='pip2build-')
    subprocess.check_call(['pip', 'install', '--no-use-wheel', '-d', tmp] + list(pkgs))
    pkgs = os.listdir(tmp)
    session = requests.Session()
    for pkg in pkgs:
        if not pkg.endswith('.tar.gz'):
            stderr('Not a downloaded tarball: %s', pkg)
            continue
        convert(session, os.path.join(tmp, pkg), pkgs)


def extract_info(pkg_info):
    pkg = ver = None
    for line in pkg_info:
        mat = re.search('^Name: (.+)$|^Version: (.+)$', line)
        if not mat:
            continue
        tpkg, tver = mat.groups()
        if tpkg:
            pkg = tpkg
        if tver:
            ver = tver
        if ver and pkg:
            return pkg, ver
    raise ValueError('Could not find Name and Version', pkg, ver, pkg_info)



def convert(session, full, pkgs):  # pylint: disable=too-many-statements,too-many-locals,too-many-branches
    base = os.path.basename(full)
    stderr('%s: converting...', base)
    deps = []
    with tarfile.open(full) as tar:
        for name in tar.getnames():
            if 'PKG-INFO' in name:
                fp = tar.extractfile(name)
                try:
                    pkg, ver = extract_info(fp)
                finally:
                    fp.close()
                break
        else:
            raise ValueError('PKG-INFO not in %s', base)
        stderr('  info: %s %s', pkg, ver)
        try:
            root = '%s-%s/%s' % (pkg, ver, pkg)
            tar.getmember(root)
        except KeyError:
            root = '%s-%s' % (pkg, ver)
            tar.getmember(root)
        try:
            mem = tar.getmember('%s-%s/%s.egg-info/requires.txt' % (pkg, ver, pkg))
        except KeyError:
            mem = None
        if mem:
            fp = tar.extractfile(mem)
            try:
                for line in fp:
                    # yes
                    mat = re.search(r'^([\w._-]+)', line)
                    if not mat:
                        continue
                    dep = mat.group(1)
                    for option in pkgs:
                        if not option.startswith('%s-' % dep.replace('-', '_')):
                            continue
                        deps.append(dep)
            finally:
                fp.close()
        if deps:
            stderr('  deps: %s', ', '.join(deps))
        else:
            stderr('  no deps')

    pkg_url = 'https://pypi.python.org/pypi/%s/json' % pkg
    resp = session.get(pkg_url)
    resp.raise_for_status()
    meta = json.loads(resp.content)
    for option in meta['releases'][ver]:
        if option['python_version'] == 'source' and option['url'].endswith('.tar.gz'):
            url = option['url']
            md5 = option['md5_digest']
            break
    else:
        raise ValueError('Cannot find source url for %s' % pkg)
    stderr('  url: %s', url)
    with open(full) as fp:
        buf = fp.read()
    digest = hashlib.md5(buf).hexdigest()
    if digest != md5:
        raise ValueError('Downloaded md5: %s != expected: %s' % (digest, md5))

    sha = hashlib.sha256()
    with open(full) as fp:
        buf = True
        while buf:
            buf = fp.read(10**6)
            sha.update(buf)
    digest = sha.hexdigest()
    with open('BUILD-%s' % pkg, 'w') as fp:
        dep_lines = deps and [
            '    deps = [',
            '\n'.join('      "@%s//:%s",' % (d, d) for d in deps),
            '    ],',
        ] or []
        fp.write('\n'.join([
            '',
            'new_http_archive(',
            '    name = "%s",' % pkg.replace('-', '_'),
            '    build_file_content = """',
            'py_library(',
            '    name = "%s",' % pkg.replace('-', '_'),
            '    srcs = glob(["**/*.py"]),',
        ] + dep_lines + [
            '    visibility = ["//visibility:public"],',
            ')',
            '""",',
            '    sha256 = "%s",' % digest,
            '    strip_prefix = "%s",' % root,
            '    urls = ["%s"],' % url,
            ')',
            '',
        ]))


if __name__ == '__main__':
    main(*sys.argv[1:])
