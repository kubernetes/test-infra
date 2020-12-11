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
import re
import subprocess
import sys
import tempfile
import time

# Import the Secret Manager client library.
# Requires `pip install google-cloud-secret-manager`
from google.cloud import secretmanager

reAutoKey = re.compile('^config-(\\d{8})$')
gcp_secret_key = "prowkubeconfigbackup"


def call(cmd, **kwargs):
    print('>>> %s' % cmd)
    return subprocess.run(
        cmd,
        check=True,
        shell=True,
        stderr=sys.stderr,
        stdout=subprocess.PIPE,
        timeout=10,  # seconds
        universal_newlines=True,
        **kwargs,
    )


def save_secret_to_gcp(project_id, data, secret_id=gcp_secret_key):
    dst = f"projects/{project_id}/secrets/{secret_id}"
    print('Saving secret to %s' % dst)
    client = secretmanager.SecretManagerServiceClient()
    secret = client.get_secret(name=dst)
    if not secret:
        secret = client.create_secret(
            request={
                "parent": f"projects/{project_id}",
                "secret_id": secret_id,
                "secret": {"replication": {"automatic": {}}},
            }
        )

    version = client.add_secret_version(
        request={"parent": secret.name, "payload": {"data": data}}
    )


def main(args):
    print(args)
    validateArgs(args)
    if args.auto:
        # We need to determine the dest key automatically.
        args.dest_key = time.strftime('config-%Y%m%d')
        if not args.src_key:
            # Also try to automatically determine the src key.
            cmd = 'kubectl --context="%s" get secret --namespace "%s" "%s" -o go-template="{{range \\$key, \\$content := .data}}{{\\$key}};{{end}}"' % (
                args.context, args.namespace, args.name)  # pylint: disable=line-too-long
            keys = call(cmd).stdout.rstrip(";").split(";")
            matches = [key for key in keys if reAutoKey.match(key)]
            matches.sort(reverse=True)
            if len(matches) == 0:
                raise ValueError(
                    'The %s/%s secret does not contain any keys matching the "config-20200730" format. Please try again with --src-key set to the most recent key. Existing keys: %s'
                    % (args.namespace, args.name, keys))  # pylint: disable=line-too-long

            args.src_key = matches[0]
        # Only enable pruning if we won't overwrite the source key.
        # This ensures that a second update on the same day will still have a
        # key to roll back to if needed.
        args.prune = args.src_key != args.dest_key
        print('Automatic mode: --src-key=%s  --dest-key=%s' % (args.src_key, args.dest_key))

    with tempfile.TemporaryDirectory() as tmpdir:
        orig = '%s/original' % (tmpdir)
        merged = '%s/merged' % (tmpdir)
        # Copy the current secret contents into a temp file.
        cmd = 'kubectl --context="%s" get secret --namespace "%s" "%s" -o go-template="{{index .data \\"%s\\"}}" | base64 -d > %s' % (
            args.context, args.namespace, args.name, args.src_key, orig)  # pylint: disable=line-too-long
        call(cmd)

        with open(orig, 'rb') as orig_file:  # Read file into bytes as it's expected format
            save_secret_to_gcp(args.project_id, orig_file.read())

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
            call('kubectl --context="%s" create secret generic --namespace "%s" "%s" --from-file="%s=%s" %s --dry-run -oyaml | kubectl --context="%s" replace -f -' %
                 (args.context, args.namespace, args.name, args.dest_key, merged, srcflag, args.context))  # pylint: disable=line-too-long
        else:
            content = ''
            with open(merged, 'r') as mergedFile:
                yamlPad = '    '
                content = yamlPad + mergedFile.read()
                content = content.replace('\n', '\n' + yamlPad)
            call('kubectl --context="%s" patch --namespace "%s" "secret/%s" --patch "stringData:\n  %s: |\n%s\n"' %
                 (args.context, args.namespace, args.name, args.dest_key, content))  # pylint: disable=line-too-long

        print('Successfully updated secret "%s/%s". The new kubeconfig is under the key "%s".' %
              (args.namespace, args.name, args.dest_key))  # pylint: disable=line-too-long
        print('Don\'t forget to update any deployments or podspecs that use the secret to reference the updated key!')  # pylint: disable=line-too-long


def validateArgs(args):
    if args.auto:
        if args.dest_key:
            raise ValueError("--dest-key must be omitted when --auto is used.")
    else:
        if not args.src_key or not args.dest_key:
            raise ValueError("--src-key and --dest-key are required unless --auto is used.")


if __name__ == '__main__':
    parser = argparse.ArgumentParser(
        description='Merges the provided kubeconfig file into a kubeconfig file living in a kubernetes secret in order to add new cluster contexts to the secret. Requires kubectl and base64.')  # pylint: disable=line-too-long
    parser.add_argument(
        '--context',
        help='The kubectl context of the cluster containing the secret.',
        required=True,
    )
    parser.add_argument(
        '--project-id',
        help='The project id which the secret is saved to.',
        required=True,
    )
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
    )
    parser.add_argument(
        '--dest-key',
        help='The destination key of the merged kubeconfig file in the k8s secret.',
    )
    parser.add_argument(
        'kubeconfig_to_merge',
        help='Filepath of the kubeconfig file to merge into the kubeconfig secret.',
    )
    parser.add_argument(
        '--prune',
        action='store_true',
        help='Remove all secret keys besides the source and dest. This should be used periodically to delete old kubeconfigs and keep the secret size under control.',  # pylint: disable=line-too-long
    )
    parser.add_argument(
        '--auto',
        action='store_true',
        help='Automatically determine --dest-key and optionally --src-key assuming keys are of the form "config-20200730". Pruning is enabled.',  # pylint: disable=line-too-long
    )

    main(parser.parse_args())
