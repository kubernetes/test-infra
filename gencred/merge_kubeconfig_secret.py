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

"""Merges a kubeconfig file into a kubeconfig file in a k8s secret giving
precedence to the secret."""

# Requirements: kubectl (pointed at the correct cluster) and base64
# Example usage:
# ./merge_kubeconfig_secret.py \
#   --name=mykube \
#   --namespace=myspace \
#   --src-key=kube-old \
#   --dest-key=kube-new \
#   kubeconfig.yaml

import argparse
import os
import subprocess
import sys
import tempfile

def call(cmd, **kwargs):
    print('>>> %s' % cmd)
    return subprocess.run(
        cmd,
        check=True,
        shell=True,
        stderr=sys.stderr,
        stdout=subprocess.PIPE,
        timeout=10,#seconds
        universal_newlines=True,
        **kwargs,
    )

def main(args):
    print(args)
    with tempfile.TemporaryDirectory() as tmpdir:
        orig = '%s/original' % (tmpdir)
        merged = '%s/merged' % (tmpdir)
        # Copy the current secret contents into a temp file.
        cmd = 'kubectl get secret --namespace "%s" "%s" -o go-template="{{index .data \\"%s\\"}}" | base64 -d > %s' % (args.namespace, args.name, args.src_key, orig) #pylint: disable=line-too-long
        call(cmd)

        # Merge the existing and new kubeconfigs into another temp file.
        env = os.environ.copy()
        env['KUBECONFIG'] = '%s:%s' % (orig, args.kubeconfig_to_merge)
        call(
            'kubectl config view --raw > %s' % (merged),
            env=env,
        )

        # Update the secret with the merged config.
        if args.prune:
            # Pruning was request. Remove all keys except for dest and src (if different from dest).
            srcflag = ''
            if args.src_key != args.dest_key:
                srcflag = '--from-file="%s=%s"' % (args.src_key, orig)
            call('kubectl create secret generic --namespace "%s" "%s" --from-file="%s=%s" %s --dry-run -oyaml | kubectl replace -f -' % (args.namespace, args.name, args.dest_key, merged, srcflag)) #pylint: disable=line-too-long
        else:
            content = ''
            with open(merged, 'r') as mergedFile:
                yamlPad = '    '
                content = yamlPad + mergedFile.read()
                content = content.replace('\n', '\n' + yamlPad)
            call('kubectl patch --namespace "%s" "secret/%s" --patch "stringData:\n  %s: |\n%s\n"' % (args.namespace, args.name, args.dest_key, content)) #pylint: disable=line-too-long

        print('Successfully updated secret %s/%s.' % (args.namespace, args.name))

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Merges the provided kubeconfig file into a kubeconfig file living in a kubernetes secret in order to add new cluster contexts to the secret. Requires kubectl and base64.') #pylint: disable=line-too-long
    parser.add_argument(
        '--name',
        help='The name of the k8s secret containing the kubeconfig file to add to.',
        default='kubeconfig',
    )
    parser.add_argument(
        '--namespace',
        help='The namespace containing the kubeconfig k8s secret to add to.',
        default='default',
    )
    parser.add_argument(
        '--src-key',
        help='The key of the source kubeconfig file in the k8s secret.',
        required=True,
    )
    parser.add_argument(
        '--dest-key',
        help='The destination key of the merged kubeconfig file in the k8s secret.',
        required=True,
    )
    parser.add_argument(
        'kubeconfig_to_merge',
        help='Filepath of the kubeconfig file to merge into the kubeconfig secret.',
    )
    parser.add_argument(
        '--prune',
        action='store_true',
        help='Remove all secret keys besides the source and dest. This should be used periodically to delete old kubeconfigs and keep the secret size under control.' #pylint: disable=line-too-long
        )

    main(parser.parse_args())
