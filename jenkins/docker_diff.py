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

"""Output the differences between two Docker images.

Usage:
  python docker_diff.py [--deep=path] <image_1> <image_2>
"""

import argparse
import json
import logging
import os
import shutil
import subprocess
import tarfile
import tempfile


def call(cmd, **kwargs):
    """run call with args."""
    logging.info('exec %s', ' '.join(cmd))
    return subprocess.call(cmd, **kwargs)


def check_call(cmd):
    """run check_call with args."""
    logging.info('exec %s', ' '.join(cmd))
    return subprocess.check_call(cmd)


def dockerfile_layers(tarball):
    '''Given a `docker save` tarball, return the layer metadata in order.'''

    layer_by_parent = {}

    for member in tarball.getmembers():
        if member.name.endswith('/json'):
            layer = json.load(tarball.extractfile(member))
            layer_by_parent[layer.get('parent')] = layer

    # assemble layers by following parent pointers
    layers = []
    parent = None  # base image has no parent
    while parent in layer_by_parent:
        layer = layer_by_parent[parent]
        layers.append(layer)
        parent = layer['id']

    return layers


def is_whiteout(fname):
    """Check if whiteout."""
    return fname.startswith('.wh.') or '/.wh.' in fname


def extract_layers(tarball, layers, outdir):
    '''Extract docker layers to a specific directory (fake a union mount).'''
    for layer in layers:
        obj = tarball.extractfile('%s/layer.tar' % layer['id'])
        with tarfile.open(fileobj=obj) as fp:
            # Complication: .wh. files indicate deletions.
            # https://github.com/docker/docker/blob/master/image/spec/v1.md
            members = fp.getmembers()
            members_good = [m for m in members if not is_whiteout(m.name)]

            fp.extractall(outdir, members_good)

            for member in members:
                name = member.name
                if is_whiteout(name):
                    path = os.path.join(outdir, name.replace('.wh.', ''))
                    if os.path.isdir(path):
                        shutil.rmtree(path)
                    elif os.path.exists(path):
                        os.unlink(path)


def docker_diff(image_a, image_b, tmpdir, deep):
    """Diff two docker images."""

    # dump images for inspection
    tf_a_path = '%s/a.tar' % tmpdir
    tf_b_path = '%s/b.tar' % tmpdir

    check_call(['docker', 'save', '-o', tf_a_path, image_a])
    check_call(['docker', 'save', '-o', tf_b_path, image_b])

    tf_a = tarfile.open(tf_a_path)
    tf_b = tarfile.open(tf_b_path)

    # find layers in order
    layers_a = dockerfile_layers(tf_a)
    layers_b = dockerfile_layers(tf_b)

    # minor optimization: skip identical layers
    common = len(os.path.commonprefix([layers_a, layers_b]))

    tf_a_out = '%s/a' % tmpdir
    tf_b_out = '%s/b' % tmpdir

    extract_layers(tf_a, layers_a[common:], tf_a_out)
    extract_layers(tf_b, layers_b[common:], tf_b_out)

    # actually compare the resulting directories

    # just show whether something changed (OS upgrades change a lot)
    call(['diff', '-qr', 'a', 'b'], cwd=tmpdir)

    if deep:
        # if requested, do a more in-depth content diff as well.
        call([
            'diff', '-rU5',
            os.path.join('a', deep),
            os.path.join('b', deep)],
             cwd=tmpdir)


def main():
    """Run docker_diff."""
    logging.basicConfig(level=logging.INFO)
    parser = argparse.ArgumentParser()
    parser.add_argument('--deep', help='Show full differences for specific directory')
    parser.add_argument('image_a')
    parser.add_argument('image_b')
    options = parser.parse_args()

    tmpdir = tempfile.mkdtemp(prefix='docker_diff_')
    try:
        docker_diff(options.image_a, options.image_b, tmpdir, options.deep)
    finally:
        shutil.rmtree(tmpdir)


if __name__ == '__main__':
    main()
