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

# USAGE: find_issues.py <github_token>
from __future__ import print_function

import sys
import json

import requests

GITHUB_API_URL = "https://api.github.com"


def do_github_graphql_request(query, token, variables=None):
    """performs a requests.put with the correct headers for a GitHub request.

    see: https://developer.github.com/v4/

    Args:
    query: the graphQL query string (excluding the variables section)
    token: a GitHub API token string
    variables: a dict of key=value variables that will be sent with the query

    Returns:
    A requests.Response object
    """
    url = "https://api.github.com/graphql"
    headers = {
        "Authorization": "bearer " + token,
    }
    data = json.dumps({"query": query, "variables": json.dumps(variables)})
    return requests.post(url, headers=headers, data=data)


def get_issues(owner, name, token, after=None):
    """returns the result of do_github_graphql_request for a repo issues query.

    This query requests the first 100 issues for a repo with the first 100
    assignee logins and labels as well as the issue title, number, state,
    creation time and the pageInfo for getting the next page of results

    Args:
    owner: the GitHub repo owner as in github.com/kubernetes/test-infra ->
            owner="kubernetes"
    name: this GitHub repo name as in github.com.kubernetes/test-infra ->
            name = "test-infra"
    token: a GitHub API token string
    after: this should be None or the endCursor from pageInfo

    Returns:
    A requests.Response object
    """
    query = ("query($owner:String!, $name:String!, $after:String) {\n"
            "  repository(owner:$owner, name:$name) {\n"
            "    issues(first:100, after:$after) {\n"
            "      nodes{\n"
            "        number\n"
            "        title\n"
            "        state\n"
            "        createdAt\n"
            "        labels(first:100){\n"
            "          nodes{\n"
            "            name\n"
            "          }\n"
            "        }\n"
            "        assignees(first:100){\n"
            "          nodes{\n"
            "            login\n"
            "          }\n"
            "        }\n"
            "      }\n"
            "      pageInfo{\n"
            "        hasNextPage\n"
            "        endCursor\n"
            "      }\n"
            "    }\n"
            "  }\n"
            "  rateLimit {\n"
            "    limit\n"
            "    cost\n"
            "    remaining\n"
            "    resetAt\n"
            "  }\n"
            "}\n")
    variables = {"owner": owner, "name": name}
    if after is not None:
        variables["after"] = after
    return do_github_graphql_request(query, token, variables)


def get_all_issues(owner, name, token, issue_func, show_progress=False):
    """gets all issues for a repo and applies issue_func to each.

    Args:
    owner: the GitHub repo owner as in github.com/kubernetes/test-infra ->
            owner="kubernetes"
    name: this GitHub repo name as in github.com.kubernetes/test-infra ->
            name = "test-infra"
    token: a GitHub API token string
    issue_func: a function that takes one argument (the json of each issue)
        this will be applied to each issue object returned by the GitHub API
    show_progress: if True then print '.' for each request made

    Raises:
    IOError: an error occurred while getting the issues
    """
    response = get_issues(owner, name, token)
    while True:
        if show_progress:
            print(".", end="")
            sys.stdout.flush()
        if response.status_code != 200:
            raise IOError("failed to fetch issues for repo: %s/%s" % (owner, name))
        response_json = response.json()
        # NOTE: this will also contain the rate limit info if we need that later
        # https://developer.github.com/v4/guides/resource-limitations/
        data = response_json["data"]
        issues = data["repository"]["issues"]["nodes"]
        for entry in issues:
            issue_func(entry)
        page_info = data["repository"]["issues"]["pageInfo"]
        if not page_info["hasNextPage"]:
            break
        response = get_issues(owner, name, token, page_info["endCursor"])


def main():
    token = sys.argv[-1]
    org = "kubernetes"
    repo = "test-infra"
    print("getting issues for: %s/%s" % (org, repo))
    # TODO: replace this with with something more useful?
    def issue_func(issue):
        print(issue)
    get_all_issues(org, repo, token, issue_func)
    print("done")


if __name__ == "__main__":
    main()
