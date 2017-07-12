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

  $ bazel run //experiment:generate_tests \
      --yaml-config-path=experiment/test_config.yaml \
      --json-config-path=jobs/config.json \
      --prow-config-path=prow/config.yaml
"""

import argparse
import hashlib
import json
import os
import ruamel.yaml as yaml


# TODO(yguo0905): Generate Prow and testgrid configurations.

PROW_CONFIG_TEMPLATE = """
    tags:
    - generated # AUTO-GENERATED; DO NOT EDIT!
    interval:
    name:
    spec:
      containers:
      - args:
        - --bare
        env:
        - name: GOOGLE_APPLICATION_CREDENTIALS
          value: /etc/service-account/service-account.json
        - name: USER
          value: prow
        - name: JENKINS_GCE_SSH_PRIVATE_KEY_FILE
          value: /etc/ssh-key-secret/ssh-private
        - name: JENKINS_GCE_SSH_PUBLIC_KEY_FILE
          value: /etc/ssh-key-secret/ssh-public
        image: gcr.io/k8s-testimages/kubekins-e2e-prow:v20170707-54693f85
        volumeMounts:
        - mountPath: /etc/service-account
          name: service
          readOnly: true
        - mountPath: /etc/ssh-key-secret
          name: ssh
          readOnly: true
      volumes:
      - name: service
        secret:
          secretName: service-account
      - name: ssh
        secret:
          defaultMode: 256
          secretName: ssh-key-secret
"""


def get_sha1_hash(data):
    """Returns the SHA1 hash of the specified data."""
    sha1_hash = hashlib.sha1()
    sha1_hash.update(data)
    return sha1_hash.hexdigest()


def substitute(job_name, lines):
    """Replace '${job_name_hash}' in lines with the SHA1 hash of job_name."""
    return [line.replace('${job_name_hash}', get_sha1_hash(job_name)[:10]) \
            for line in lines]


def get_envs(desc, field):
    """Returns a list of envs for the given field."""
    header = ['', '# The %s configurations.' % desc]
    if not field:
        return header
    return header + field.get('envs', [])


def get_args(job_name, field):
    """Returns a list of args for the given field."""
    if not field:
        return []
    return substitute(job_name, field.get('args', []))


def get_project_id(job_name):
    """Returns the project id generated from the given job_name."""
    return 'k8s-test-%s' % get_sha1_hash(job_name)[:10]


def get_job_def(env_filename, args, sig_owners):
    """Returns the job definition given the env_filename and the args."""
    return {
        'scenario': 'kubernetes_e2e',
        'args': ['--env-file=%s' % env_filename] + args,
        'sigOwners': sig_owners or ['UNNOWN'],
        # Indicates that this job definition is auto-generated.
        'tags': ['generated'],
        '_comment': 'AUTO-GENERATED - DO NOT EDIT.'
    }


def write_env_file(output_dir, job_name, envs):
    """Writes envs into a file in output_dir, and returns the file name."""
    output_file = os.path.join(output_dir, '%s.env' % job_name)
    with open(output_file, 'w') as fp:
        fp.write('\n'.join(envs))
        fp.write('\n')
    return output_file


def write_job_defs_file(output_dir, job_defs):
    """Writes the job definitions into a file in output_dir."""
    output_file = os.path.join(output_dir, 'config.json')
    with open(output_file, 'w') as fp:
        json.dump(
            job_defs, fp, sort_keys=True, indent=2, separators=(',', ': '))
        fp.write('\n')


def write_prow_configs_file(output_dir, job_defs):
    """Writes the Prow configurations into a file in output_dir."""
    output_file = os.path.join(output_dir, 'config.yaml')
    with open(output_file, 'w') as fp:
        yaml.dump(
            job_defs, fp, Dumper=yaml.RoundTripDumper, width=float("inf"))
        fp.write('\n')


def generate_envs(job_name, common, cloud_provider, image, k8s_version,
                  test_suite, job):
    """Returns a list of envs fetched from the given fields."""
    envs = ['# AUTO-GENERATED - DO NOT EDIT.']
    envs.extend(get_envs('common', common))
    envs.extend(get_envs('cloud provider', cloud_provider))
    envs.extend(get_envs('image', image))
    envs.extend(get_envs('k8s version', k8s_version))
    envs.extend(get_envs('test suite', test_suite))
    envs.extend(get_envs('job', job))
    if not any(e.strip().startswith('PROJECT=') for e in envs):
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


def generate_prow_config(job_name, test_suite, job):
    """Returns the Prow config for the job from the given fields."""
    prow_config = yaml.round_trip_load(PROW_CONFIG_TEMPLATE)
    prow_config['name'] = job_name
    prow_config['interval'] = job['interval']
    # Assumes that the value in --timeout is of minutes.
    timeout = int(next(
        x[10:-1] for x in test_suite['args'] if x.startswith('--timeout=')))
    # Prow timeout = job timeout + 20min
    prow_config['spec']['containers'][0]['args'].append(
        '--timeout=%d' % (timeout + 20))
    return prow_config


def for_each_job(job_name, common, cloud_providers, images, k8s_versions,
                 test_suites, jobs):
    """Returns the envs list and the args list for each test job."""
    fields = job_name.split('-')
    if len(fields) != 7:
        raise ValueError('Expected 7 fields in job name', job_name)

    cloud_provider_name = fields[3]
    image_name = fields[4]
    k8s_version_name = fields[5][3:]
    test_suite_name = fields[6]

    envs = generate_envs(
        job_name,
        common,
        cloud_providers[cloud_provider_name],
        images[image_name],
        k8s_versions[k8s_version_name],
        test_suites[test_suite_name],
        jobs[job_name])
    args = generate_args(
        job_name,
        common,
        cloud_providers[cloud_provider_name],
        images[image_name],
        k8s_versions[k8s_version_name],
        test_suites[test_suite_name],
        jobs[job_name])
    prow = generate_prow_config(
        job_name,
        test_suites[test_suite_name],
        jobs[job_name])
    return envs, args, prow


def remove_generated_jobs(json_config):
    """Removes all the generated job configs and their env files."""
    # TODO(yguo0905): Remove the generated env files as well.
    return {
        name: job_def for (name, job_def) in json_config.items()
        if 'generated' not in job_def.get('tags', [])}


def remove_generated_prow_configs(prow_config):
    """Removes all the generated Prow configurations."""
    # TODO(yguo0905): Handle non-periodics jobs.
    prow_config['periodics'] = [
        job for job in prow_config.get('periodics', [])
        if 'generated' not in job.get('tags', [])]


def main(json_config_path, yaml_config_path, prow_config_path, output_dir):
    """Creates test job definitions.

    Converts the test configurations in yaml_config_path to the job definitions
    in json_config_path and the env files in output_dir.
    """
    # TODO(yguo0905): Validate the configurations from yaml_config_path.

    with open(json_config_path) as fp:
        json_config = json.load(fp)
    json_config = remove_generated_jobs(json_config)

    with open(prow_config_path) as fp:
        prow_config = yaml.round_trip_load(fp, preserve_quotes=True)
    remove_generated_prow_configs(prow_config)

    with open(yaml_config_path) as fp:
        yaml_config = yaml.safe_load(fp)

    for job_name, _ in yaml_config['jobs'].items():
        # Get the envs and args for each job defined under "jobs".
        envs, args, prow = for_each_job(
            job_name,
            yaml_config['common'],
            yaml_config['cloudProviders'],
            yaml_config['images'],
            yaml_config['k8sVersions'],
            yaml_config['testSuites'],
            yaml_config['jobs'])
        # Write the extracted envs into an env file for the job.
        env_filename = write_env_file(output_dir, job_name, envs)
        # Add the job to the definitions.
        sig_owners = yaml_config['jobs'][job_name].get('sigOwners')
        json_config[job_name] = get_job_def(env_filename, args, sig_owners)
        prow_config['periodics'].append(prow)

    # Write the job definitions to config.json.
    write_job_defs_file(output_dir, json_config)
    write_prow_configs_file('prow', prow_config)


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        description='Create test definitions from the given yaml config')
    PARSER.add_argument('--yaml-config-path', help='Path to config.yaml')
    PARSER.add_argument('--json-config-path', help='Path to config.json')
    PARSER.add_argument('--prow-config-path', help='Path to the Prow config')
    PARSER.add_argument(
        '--output-dir', help='Env files output dir', default='jobs')
    ARGS = PARSER.parse_args()

    main(
        ARGS.json_config_path,
        ARGS.yaml_config_path,
        ARGS.prow_config_path,
        ARGS.output_dir)
