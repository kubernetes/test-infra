#!/usr/bin/env python

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

"""Create e2e test definitions.

Usage example:

  In $GOPATH/src/k8s.io/test-infra,

  $ ./experiment/generate_tests.py --config-path=experiment/test_config.yaml \
    --output-dir=jobs/generated

"""

import argparse
import hashlib
import json
import os
import yaml

# pylint: disable=too-many-branches,too-many-statements,too-many-locals


# TODO(yguo0905): Generate Prow and testgrid configurations.

def get_sha1_hash(data):
    sha1_hash = hashlib.sha1()
    sha1_hash.update(data)
    return sha1_hash.hexdigest()


def substitute(job_name, lines):
    return [line.replace('${job_name_hash}', get_sha1_hash(job_name)[:10]) \
            for line in lines]


def get_envs(desc, field):
    """Returns a list of envs from the given field, or an empty list if the
    field is None or the field does not contain the key "envs"."""

    result = ['', '# The %s configurations.' % desc]
    if field is None or field.get('envs') is None:
        return result
    return result + field['envs']


def get_args(job_name, field):
    """Returns a list of args from the given field, and an empty list if the
    field is None or the field does not contain the key "args"."""

    if field is None or field.get('args') is None:
        return []
    return substitute(job_name, field['args'])


def get_project_id(job_name):
    """Returns the project id generated from the given job_name."""

    return 'k8s-test-%s' % get_sha1_hash(job_name)[:10]


def get_job_def(env_filename, args, sig_owners):
    """Returns the job definition given the env_filename and the args."""

    result = dict()
    result['scenario'] = 'kubernetes_e2e'
    result['args'] = []
    result['args'].append('--env-file=%s' % env_filename)
    result['args'].extend(args)
    result['sigOwners'] = ['UNKNOWN'] if sig_owners is None else sig_owners
    return result


def write_env_file(output_dir, job_name, envs):
    """Writes envs into a file in output_dir, and returns the file name."""

    output_file = os.path.join(output_dir, job_name + '.env')
    with open(output_file, 'w') as fp:
        for env in envs:
            fp.write(env + '\n')
    return output_file


def write_job_defs_file(output_dir, job_defs):
    """Writes job definitions into a file in output_dir."""

    output_file = os.path.join(output_dir, 'config.json')
    with open(output_file, 'w') as fp:
        json.dump(job_defs, fp, sort_keys=True, indent=4,
                  separators=(',', ': '))
        fp.write('\n')


def generate_envs(job_name, common, cloud_provider, image, k8s_version,
                  test_suite, job):
    """Returns a list of envs fetched from the given fields."""

    envs = ['# Auto-generated; DO NOT MODIFY.']
    envs.extend(get_envs('common', common))
    envs.extend(get_envs('cloud provider', cloud_provider))
    envs.extend(get_envs('image', image))
    envs.extend(get_envs('k8s version', k8s_version))
    envs.extend(get_envs('test suite', test_suite))
    envs.extend(get_envs('job', job))
    for env in envs:
        if env.strip().startswith('PROJECT='):
            break
    else:
        envs.extend(['', 'PROJECT=%s' % get_project_id(job_name)])
    return envs


def generate_args(job_name, common, cloud_provider, image, k8s_version,
                  test_suite, job):
    """Returns a list of args fetched from the given fields."""

    args = []
    args.extend(get_args(job_name, common))
    args.extend(get_args(job_name, cloud_provider))
    args.extend(get_args(job_name, image))
    args.extend(get_args(job_name, k8s_version))
    args.extend(get_args(job_name, test_suite))
    args.extend(get_args(job_name, job))
    return args


def for_each_job(job_name, common, cloud_providers, images, k8s_versions,
                 test_suites, jobs):
    '''Returns the list of envs and args for each the job with the given
    job_name.'''

    fields = job_name.split('-')
    if len(fields) != 5:
        raise ValueError('Expected 5 fields in job name', job_name)

    cloud_provider_name = fields[1]
    image_name = fields[2]
    k8s_version_name = fields[3][3:]
    test_suite_name = fields[4]

    envs = generate_envs(job_name,
                         common,
                         cloud_providers[cloud_provider_name],
                         images[image_name],
                         k8s_versions[k8s_version_name],
                         test_suites[test_suite_name],
                         jobs[job_name])
    args = generate_args(job_name,
                         common,
                         cloud_providers[cloud_provider_name],
                         images[image_name],
                         k8s_versions[k8s_version_name],
                         test_suites[test_suite_name],
                         jobs[job_name])
    return envs, args


def main(config_path, output_dir):
    '''Creates test job definitions for the jobs and their configs from the
    config_path and writes them to output_dir.'''

    # TODO(yguo0905): Validate the configurations from config_path.

    with open(config_path) as fp:
        doc = yaml.safe_load(fp)

    # TODO(yguo0905): Create output_dir if it does not exist, or remove
    # existing configs in output_dir otherwise.

    job_defs = dict()
    for job_name, _ in doc['jobs'].items():
        # Get the envs and args for each job defined under "jobs".
        envs, args = for_each_job(job_name,
                                  doc['common'],
                                  doc['cloudProviders'],
                                  doc['images'],
                                  doc['k8sVersions'],
                                  doc['testSuites'],
                                  doc['jobs'])
        # Write the extacted envs into an env file for the job.
        env_filename = write_env_file(output_dir, job_name, envs)
        # Add the job to the definitions.
        sig_owners = doc['jobs'][job_name].get('sigOwners')
        job_defs[job_name] = get_job_def(env_filename, args, sig_owners)

    # Write the job definitions to config.yaml.
    write_job_defs_file(output_dir, job_defs)


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        description='Create test definitions from the given config')
    PARSER.add_argument(
        '--config-path',
        help='Path to config.yaml',
        default=None)
    PARSER.add_argument(
        '--output-dir',
        help='Path to output dir',
        default=None)
    ARGS = PARSER.parse_args()

    main(ARGS.config_path, ARGS.output_dir)
