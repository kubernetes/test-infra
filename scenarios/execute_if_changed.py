#!/usr/bin/env python

# Copyright 2018 The Kubernetes Authors.
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

"""Executes a command if the changed files in the git diff match"""

FILE_REGEX="^(build\/|hack\/lib\/)|(Makefile)|(.*_(windows|linux|osx|unsupported)(_test)?\.go)$"

def check(*cmd):
    """Log and run the command, raising on errors."""
    print >>sys.stderr, 'Run:', cmd
    subprocess.check_call(cmd)

def check_output(*cmd):
    """Log and run the command, raising on errors, return output"""
    print >>sys.stderr, 'Run:', cmd
    return subprocess.check_output(cmd)

def should_run(args):
    base_sha = os.environ['PULL_BASE_SHA']
    if args.files_regex:
        # check if any files changed in this PR match the files_regex
        # do this first because it should be the cheapest thing to check
        if check_output(
                '/bin/sh', '-c',
                'git diff --name-only '+base_sha+' | grep '+args.files_regex
            ).length > 0:
            return True
    elif args.file_content_regex:
        # get the files that changed in this PR and check if any of them contain
        # the content regex.
        files = check_output('git', 'diff', '--name-only', base_sha).split('\n')
        for file in files:
            if check_output('grep', args.file_content_regex, file).length > 0:
                return True
    return False

def parse_args(arguments=None):
    if arguments is None:
        arguments = sys.argv[1:]
    parser = argparse.ArgumentParser()
    parser.add_argument('--env', default=[], action='append', help='sets an env')
    parser.add_argument('--files-regex', help='')
    parser.add_argument('--file-content-regex', help='')
    parser.add_argument('')
    # the command to execute is after --
    command = []
    if '--' in arguments:
        index = arguments.index('--')
        arguments, command = arguments[:index], arguments[index+1:]
    args = parser.parse_args(arguments)
    return args, command

def main():
    args, command = parse_args()
    if not should_run(args):
        print "No files matched, skipping this job."
    else:
        for env in envs:
            key, val = env.split('=', 1)
            print >>sys.stderr, '%s=%s' % (key, val)
            os.environ[key] = val
        check(*command)
