# ghactivity

`ghactivity` is an experimental tool which reports details of issues and PRs
for a particular GitHub user on a month-by-month basis.

Several flags are expected:

- `--github-token`: a GitHub API token to use for GitHub queries
- `--author`: the GitHub user for which to generate this report
- `--start`: the starting month for the repot, given in the format `Dec 2006`
- `--end`: the ending month for the report, given in the format `Feb 2006`.
  Defaults to today.

Example report for @ixdy:
```console
$ bazel run //experiment/ghactivity -- -github-token=$GHTOKEN --author=ixdy --start="Dec 2018" --end="Dec 2018"
...
GitHub activity report for ixdy

December 2018
==============
Created 2 Issues and 11 PRs

[PR] support bazel 0.20.0+ by using starlark http rules and updating rules_go (kubernetes/kubernetes#71675)
  Created 3 Dec 2018
  13 comments, 0 review comments
  2 commits, +7/-4 lines, 1 changed files
  MERGED 4 Dec 2018
[PR] Fix build for bazel 0.20.0+ (kubernetes/repo-infra#92)
  Created 4 Dec 2018
  3 comments, 0 review comments
  1 commits, +4/-2 lines, 1 changed files
  MERGED 4 Dec 2018
[PR] Update bazel to 0.18.1 for release-1.12 branch (kubernetes/test-infra#10350)
  Created 5 Dec 2018
  5 comments, 0 review comments
  3 commits, +813/-813 lines, 86 changed files
  MERGED 5 Dec 2018
[Issue] deck should degrade gracefully when tide is unavailable (kubernetes/test-infra#10351)
  Created 5 Dec 2018
  4 comments
  OPEN (last update 22 Jan 2019)
[PR] prow pod-utils/clone: make git checkouts more reproducible (kubernetes/test-infra#10377)
  Created 6 Dec 2018
  11 comments, 15 review comments
  3 commits, +274/-51 lines, 3 changed files
  MERGED 26 Dec 2018
[Issue] https broken on docs.k8s.io and docs.kubernetes.io redirectors (kubernetes/k8s#160)
  Created 9 Dec 2018
  4 comments
  CLOSED 11 Jan 2019
[PR] Automated cherry pick of #72035 and #72084: bump golang to 1.11.4 (CVE-2018-16875) (kubernetes/kubernetes#72071)
  Created 15 Dec 2018
  25 comments, 0 review comments
  3 commits, +10/-10 lines, 5 changed files
  MERGED 29 Dec 2018
[PR] Bump golang to 1.10.7 (CVE-2018-16875) (kubernetes/kubernetes#72072)
  Created 15 Dec 2018
  9 comments, 0 review comments
  1 commits, +8/-8 lines, 5 changed files
  MERGED 19 Dec 2018
[PR] Bump golang to 1.10.7 (CVE-2018-16875) (kubernetes/kubernetes#72074)
  Created 15 Dec 2018
  11 comments, 0 review comments
  1 commits, +24/-5 lines, 4 changed files
  MERGED 19 Dec 2018
[PR] Update to go1.11.4 (kubernetes/kubernetes#72084)
  Created 15 Dec 2018
  9 comments, 0 review comments
  1 commits, +6/-6 lines, 4 changed files
  MERGED 19 Dec 2018
[PR] Update to golang 1.10.7 and 1.11.4 (kubernetes/test-infra#10442)
  Created 15 Dec 2018
  5 comments, 0 review comments
  1 commits, +12/-12 lines, 6 changed files
  MERGED 16 Dec 2018
[PR] Update to kubekins-test v20181218-db74ab3f4 (kubernetes/test-infra#10464)
  Created 18 Dec 2018
  6 comments, 2 review comments
  1 commits, +9/-9 lines, 4 changed files
  MERGED 18 Dec 2018
[PR] The release-1.12 branch requires bazel 0.17.1+ now (kubernetes/kubernetes#72262)
  Created 21 Dec 2018
  2 comments, 0 review comments
  1 commits, +1/-1 lines, 1 changed files
  MERGED 21 Dec 2018
```