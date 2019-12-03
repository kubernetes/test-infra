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

"""Clean up resources from gcp projects. """

import argparse
import collections
import datetime
import json
import os
import subprocess
import sys
import threading

# A resource that need to be cleared.
Resource = collections.namedtuple(
    'Resource', 'api_version group name subgroup condition managed tolerate bulk_delete')
DEMOLISH_ORDER = [
    # [WARNING FROM KRZYZACY] : TOUCH THIS WITH CARE!
    # ORDER REALLY MATTERS HERE!

    # compute resources
    Resource('', 'compute', 'instances', None, 'zone', None, False, True),
    Resource('', 'compute', 'addresses', None, 'region', None, False, True),
    Resource('', 'compute', 'disks', None, 'zone', None, False, True),
    Resource('', 'compute', 'disks', None, 'region', None, False, True),
    Resource('', 'compute', 'firewall-rules', None, None, None, False, True),
    Resource('', 'compute', 'routes', None, None, None, False, True),
    Resource('', 'compute', 'forwarding-rules', None, None, None, False, True),
    Resource('beta', 'compute', 'forwarding-rules', None, 'region', None, False, True),
    Resource('', 'compute', 'target-http-proxies', None, None, None, False, True),
    Resource('beta', 'compute', 'target-http-proxies', None, 'region', None, False, True),
    Resource('', 'compute', 'target-https-proxies', None, None, None, False, True),
    Resource('beta', 'compute', 'target-https-proxies', None, 'region', None, False, True),
    Resource('', 'compute', 'target-tcp-proxies', None, None, None, False, True),
    Resource('beta', 'compute', 'target-tcp-proxies', None, 'region', None, False, True),
    Resource('', 'compute', 'target-tcp-proxies', None, None, None, False, True),
    Resource('beta', 'compute', 'target-tcp-proxies', None, 'region', None, False, True),
    Resource('', 'compute', 'ssl-certificates', None, None, None, False, True),
    Resource('beta', 'compute', 'ssl-certificates', None, 'region', None, False, True),
    Resource('', 'compute', 'url-maps', None, None, None, False, True),
    Resource('beta', 'compute', 'url-maps', None, 'region', None, False, True),
    Resource('', 'compute', 'backend-services', None, 'region', None, False, True),
    Resource('', 'compute', 'target-pools', None, 'region', None, False, True),
    Resource('', 'compute', 'health-checks', None, None, None, False, True),
    Resource('beta', 'compute', 'health-checks', None, 'region', None, False, True),
    Resource('', 'compute', 'http-health-checks', None, None, None, False, True),
    Resource('', 'compute', 'instance-groups', None, 'zone', 'Yes', False, True),
    Resource('', 'compute', 'instance-groups', None, 'zone', 'No', False, True),
    Resource('', 'compute', 'instance-templates', None, None, None, False, True),
    Resource('', 'compute', 'sole-tenancy', 'node-groups', 'zone', None, False, True),
    Resource('', 'compute', 'sole-tenancy', 'node-templates', 'region', None, False, True),
    Resource('beta', 'compute', 'network-endpoint-groups', None, 'zone', None, False, False),
    Resource('', 'compute', 'networks', 'subnets', 'region', None, True, True),
    Resource('', 'compute', 'networks', None, '', None, False, True),
    Resource('', 'compute', 'routes', None, None, None, False, True),
    Resource('', 'compute', 'routers', None, 'region', None, False, True),

    # logging resources
    Resource('', 'logging', 'sinks', None, None, None, False, False),
]


def log(message):
    """ print a message if --verbose is set. """
    if ARGS.verbose:
        tss = "[" + str(datetime.datetime.now()) + "] "
        print tss + message + '\n'


def base_command(resource):
    """ Return the base gcloud command with api_version, group and subgroup.

    Args:
        resource: Definition of a type of gcloud resource.
    Returns:
        list of base commands of gcloud .
    """

    base = ['gcloud']
    if resource.api_version:
        base += [resource.api_version]
    base += [resource.group, '-q', resource.name]
    if resource.subgroup:
        base.append(resource.subgroup)
    return base


def validate_item(item, age, resource, clear_all):
    """ Validate if an item need to be cleaned.

    Args:
        item: a gcloud resource item from json format.
        age: Time cutoff from the creation of a resource.
        resource: Definition of a type of gcloud resource.
        clear_all: If need to clean regardless of timestamp.
    Returns:
        True if object need to be cleaned, False otherwise.
    Raises:
        ValueError if json result from gcloud is invalid.
    """

    if resource.managed:
        if 'isManaged' not in item:
            raise ValueError(resource.name, resource.managed)
        if resource.managed != item['isManaged']:
            return False

    # clears everything without checking creationTimestamp
    if clear_all:
        return True

    if 'creationTimestamp' not in item:
        raise ValueError('missing key: creationTimestamp - %r' % item)

    # Unify datetime to use utc timezone.
    created = datetime.datetime.strptime(item['creationTimestamp'], '%Y-%m-%dT%H:%M:%S')
    log('Found %r(%r), %r, created time = %r' %
        (resource.name, resource.subgroup, item['name'], item['creationTimestamp']))
    if created < age:
        log('Added to janitor list: %r(%r), %r' %
            (resource.name, resource.subgroup, item['name']))
        return True
    return False


def collect(project, age, resource, filt, clear_all):
    """ Collect a list of resources for each condition (zone or region).

    Args:
        project: The name of a gcp project.
        age: Time cutoff from the creation of a resource.
        resource: Definition of a type of gcloud resource.
        filt: Filter clause for gcloud list command.
        clear_all: If need to clean regardless of timestamp.
    Returns:
        A dict of condition : list of gcloud resource object.
    Raises:
        ValueError if json result from gcloud is invalid.
        subprocess.CalledProcessError if cannot list the gcloud resource
    """

    col = collections.defaultdict(list)

    # TODO(krzyzacy): logging sink does not have timestamp
    #                 don't even bother listing it if not clear_all
    if resource.name == 'sinks' and not clear_all:
        return col

    cmd = base_command(resource)
    cmd.extend([
        'list',
        '--format=json(name,creationTimestamp.date(tz=UTC),zone,region,isManaged)',
        '--filter=%s' % filt,
        '--project=%s' % project])
    log('%r' % cmd)

    # TODO(krzyzacy): work around for alpha API list calls
    try:
        items = subprocess.check_output(cmd)
    except subprocess.CalledProcessError:
        if resource.tolerate:
            return col
        raise

    for item in json.loads(items):
        log('parsing item: %r' % item)

        if 'name' not in item:
            raise ValueError('missing key: name - %r' % item)

        if resource.condition and resource.condition in item:
            colname = item[resource.condition]
            log('looking for items in %s=%s' % (resource.condition, colname))
        else:
            colname = ''

        if validate_item(item, age, resource, clear_all):
            col[colname].append(item['name'])
    return col

def asyncCall(cmd, tolerate, name, errs, lock, hide_output):
    log('Call %r' % cmd)
    try:
        if hide_output:
            FNULL = open(os.devnull, 'w')
            subprocess.check_call(cmd, stdout=FNULL)
        else:
            subprocess.check_call(cmd)
    except subprocess.CalledProcessError as exc:
        if not tolerate:
            with lock:
                errs.append(exc)
        print >> sys.stderr, 'Error try to delete resources %s: %r' % (name, exc)

def clear_resources(project, cols, resource, rate_limit):
    """Clear a collection of resource, from collect func above.

    Args:
        project: The name of a gcp project.
        cols: A dict of collection of resource.
        resource: Definition of a type of gcloud resource.
        rate_limit: how many resources to delete per gcloud delete call
    Returns:
        0 if no error
        > 0 if deletion command fails
    """
    errs = []
    threads = list()
    lock = threading.Lock()

    # delete one resource at a time, if there's no api support
    # aka, logging sinks for example
    if not resource.bulk_delete:
        rate_limit = 1

    for col, items in cols.items():
        if ARGS.dryrun:
            log('Resource type %r(%r) to be deleted: %r' %
                (resource.name, resource.subgroup, list(items)))
            continue

        manage_key = {'Yes': 'managed', 'No': 'unmanaged'}

        # construct the customized gcloud command
        base = base_command(resource)
        if resource.managed:
            base.append(manage_key[resource.managed])
        base.append('delete')
        base.append('--project=%s' % project)

        condition = None
        if resource.condition:
            if col:
                condition = '--%s=%s' % (resource.condition, col)
            else:
                condition = '--global'

        log('going to delete %d %s' % (len(items), resource.name))
        # try to delete at most $rate_limit items at a time
        for idx in xrange(0, len(items), rate_limit):
            clean = items[idx:idx + rate_limit]
            cmd = base + list(clean)
            if condition:
                cmd.append(condition)
            thread = threading.Thread(
                target=asyncCall, args=(cmd, resource.tolerate, resource.name, errs, lock, False))
            threads.append(thread)
            log('start a new thread, total %d' % len(threads))
            thread.start()

    log('Waiting for all %d thread to finish' % len(threads))
    for thread in threads:
        thread.join()
    return len(errs)


def clean_gke_cluster(project, age, filt):
    """Clean up potential leaking gke cluster"""

    # a cluster can be created in one of those three endpoints
    endpoints = [
        'https://test-container.sandbox.googleapis.com/',  # test
        'https://staging-container.sandbox.googleapis.com/',  # staging
        'https://container.googleapis.com/',  # prod
    ]

    errs = []

    for endpoint in endpoints:
        threads = list()
        lock = threading.Lock()

        os.environ['CLOUDSDK_API_ENDPOINT_OVERRIDES_CONTAINER'] = endpoint
        log("checking endpoint %s" % endpoint)
        cmd = [
            'gcloud', 'container', '-q', 'clusters', 'list',
            '--project=%s' % project,
            '--filter=%s' % filt,
            '--format=json(name,createTime,zone)'
        ]
        log('running %s' % cmd)

        output = ''
        try:
            output = subprocess.check_output(cmd)
        except subprocess.CalledProcessError as exc:
            # expected error
            log('Cannot reach endpoint %s with %r, continue' % (endpoint, exc))
            continue

        for item in json.loads(output):
            log('cluster info: %r' % item)
            if 'name' not in item or 'createTime' not in item or 'zone' not in item:
                raise ValueError('name, createTime and zone must present: %r' % item)

            # The raw createTime string looks like 2017-08-30T18:33:14+00:00
            # Which python 2.7 does not support timezones.
            # Since age is already in UTC time we'll just strip the timezone part
            item['createTime'] = item['createTime'].split('+')[0]
            created = datetime.datetime.strptime(
                item['createTime'], '%Y-%m-%dT%H:%M:%S')

            if created < age:
                log('Found stale gke cluster %r in %r, created time = %r' %
                    (item['name'], endpoint, item['createTime']))
                delete = [
                    'gcloud', 'container', '-q', 'clusters', 'delete',
                    item['name'],
                    '--project=%s' % project,
                    '--zone=%s' % item['zone'],
                ]
                thread = threading.Thread(
                    target=asyncCall, args=(delete, False, item['name'], errs, lock, True))
                threads.append(thread)
                log('start a new thread, total %d' % len(threads))
                thread.start()

        log('Waiting for all %d thread to finish in %s' % (len(threads), endpoint))
        for thread in threads:
            thread.join()

    return len(errs) > 0


def activate_service_account(service_account):
    print '[=== Activating service_account %s ===]' % service_account
    cmd = [
        'gcloud', 'auth', 'activate-service-account',
        '--key-file=%s' % service_account,
    ]
    log('running %s' % cmd)

    try:
        subprocess.check_call(cmd)
    except subprocess.CalledProcessError:
        print >> sys.stderr, 'Error try to activate service_account: %s' % service_account
        return 1
    return 0


def main(project, days, hours, filt, rate_limit, service_account):
    """ Clean up resources from a gcp project based on it's creation time

    Args:
        project: The name of a gcp project.
        days/hours: days/hours of maximum lifetime of a gcp resource.
        filt: Resource instance filters when query.
    Returns:
        0 if no error
        1 if list or delete command fails
    """

    print '[=== Start Janitor on project %r ===]' % project
    err = 0
    age = datetime.datetime.utcnow() - datetime.timedelta(days=days, hours=hours)
    clear_all = (days is 0 and hours is 0)

    if service_account:
        err |= activate_service_account(service_account)

    if not err:
        for res in DEMOLISH_ORDER:
            log('Try to search for %r with condition %r' % (res.name, res.condition))
            try:
                col = collect(project, age, res, filt, clear_all)
                if col:
                    err |= clear_resources(project, col, res, rate_limit)
            except (subprocess.CalledProcessError, ValueError):
                err |= 1  # keep clean the other resource
                print >> sys.stderr, 'Fail to list resource %r from project %r' \
                                     % (res.name, project)

        # try to clean leaking gke cluster
        try:
            err |= clean_gke_cluster(project, age, filt)
        except ValueError:
            err |= 1  # keep clean the other resource
            print >> sys.stderr, 'Fail to clean up cluster from project %r' % project

    print '[=== Finish Janitor on project %r with status %r ===]' % (project, err)
    sys.exit(err)


if __name__ == '__main__':
    PARSER = argparse.ArgumentParser(
        description='Clean up resources from an expired project')
    PARSER.add_argument('--project', help='Project to clean', required=True)
    PARSER.add_argument(
        '--days', type=int,
        help='Clean items more than --days old (added to --hours)')
    PARSER.add_argument(
        '--hours', type=float,
        help='Clean items more than --hours old (added to --days)')
    PARSER.add_argument(
        '--filter',
        default='name !~ ^default',
        help='Filter down to these instances')
    PARSER.add_argument(
        '--dryrun',
        default=False,
        action='store_true',
        help='List but not delete resources')
    PARSER.add_argument(
        '--ratelimit', type=int, default=50,
        help='Max number of resources to bulk clear in one gcloud delete call')
    PARSER.add_argument(
        '--verbose', action='store_true',
        help='Get full janitor output log')
    PARSER.add_argument(
        '--service_account',
        help='GCP service account',
        default=os.environ.get("GOOGLE_APPLICATION_CREDENTIALS", None))
    ARGS = PARSER.parse_args()

    # We want to allow --days=0 and --hours=0, so check against None instead.
    if ARGS.days is None and ARGS.hours is None:
        print >> sys.stderr, 'must specify --days and/or --hours'
        sys.exit(1)

    main(ARGS.project, ARGS.days or 0, ARGS.hours or 0, ARGS.filter,
         ARGS.ratelimit, ARGS.service_account)
