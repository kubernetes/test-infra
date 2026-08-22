# test-coverage

`test-coverage` is a CLI tool that reports how thoroughly the Kubernetes
`e2e` and `e2e_node` Ginkgo test suites are actually exercised by the
periodic Prow jobs defined in this repository, based on real job run
results.

For each test in the chosen suite, it shows how many times it was executed
(and how often it passed) across all matching periodic jobs over a recent
time window, both in total and broken down per job. Tests that were never
executed by any matching job are listed separately at the end of the
report.

## How it works

1. **List the tests.** It runs
   `go test -v <test package> -args --list-tests` against the given
   `kubernetes/kubernetes` checkout (`./test/e2e` or `./test/e2e_node`,
   depending on `-e2e-suite`) to get the authoritative list of Ginkgo test
   names.
2. **List the jobs.** It loads the Prow configuration
   (`config/prow/config.yaml` plus the job directory given via `-job-dir`)
   using `sigs.k8s.io/prow/pkg/config`, and collects all periodic jobs
   whose name matches `-job-filter`. Jobs that don't test
   `kubernetes/kubernetes` at `master` (e.g. jobs testing another repo, or
   jobs pinned to a release branch such as `release-1.33`) are filtered
   out automatically, since their results wouldn't be meaningful for the
   current suite.
3. **Find recent runs, without listing everything.** Job results are
   read directly from the public `gs://kubernetes-ci-logs` GCS bucket via
   its anonymous REST API (no credentials needed). Build IDs are
   monotonically increasing, snowflake-like numbers, so for each job the
   tool lists `logs/<job>/` (one HTTP request), sorts the build IDs
   numerically descending, and then walks them **newest first**, fetching
   only their small `started.json` file to check the run's start time.
   As soon as it encounters a run older than `-age`, it stops —
   it does not need to look at (or download artifacts for) any older
   runs. This keeps the number of GCS requests proportional to the
   number of *recent* runs of each job, not to the job's entire history.
   Finding the recent runs of different jobs is independent work, so it
   is done concurrently, using up to `-workers` goroutines at a time.
4. **Parse JUnit results.** For each recent run, it downloads the
   `junit_*.xml` files under that run's `artifacts/` directory and parses
   them (using a copy of
   `k8s.io/kubernetes/third_party/forked/gotestsum/junitxml`). Which
   suite a run actually executed is determined from the top-level
   `<testsuite name="...">` element itself (e.g. `"E2eNode Suite"` or
   `"Kubernetes e2e suite"`), not from individual test cases; this also
   works for runs that executed the right test binary but ended up
   running zero tests. A `<testsuite>` element without a `name`
   attribute (for example the Go code coverage report
   `junit_coverage.xml` produced by some jobs) is treated as suite
   `"unknown"`. Runs of a job are processed **newest first, one job at a
   time** (all runs of a job are handled by the same worker), and as
   soon as a run is found whose JUnit files belong to a different suite,
   the rest of that job's (older) runs are skipped without being
   downloaded, since a job that doesn't run the right suite is very
   unlikely to have done so in the past; this is also why only that one
   run counts towards the "number of runs found" in the report for such
   a job. However, if a run has **no JUnit files at all** (e.g. a build
   or unit-test job that produces no Ginkgo output), nothing can be
   concluded about which suite it ran, so this doesn't stop the job:
   all of its runs within `-age` have to be downloaded and checked,
   since any of them (even an older one) might turn out to contain
   JUnit data after all. Such jobs are reported with suite `"none"` and
   0 executed tests. If a job has **no runs at all** within `-age` (e.g.
   because it doesn't run that often, or hasn't run recently), its
   suite is reported as `"unknown"` instead, since in that case not
   even one run was available to inspect. Skipped test cases are
   ignored; the rest are recorded as passed or failed based on the
   presence of a `<failure>` element. Downloading and analyzing the
   JUnit results of different jobs' runs is independent work, so it is
   done concurrently, using up to `-workers` goroutines at a time.
5. **Report.** Finally, it prints a `text/tabwriter`-aligned report:
   the list of jobs analyzed with how many runs were found for each and
   which suite ("e2e", "e2e_node", the raw JUnit suite name for anything
   else, "none" for a job whose runs had no JUnit data at all, or
   "unknown" for a job with no runs found at all within `-age`) they
   were detected to run, then for every executed test its total/per-job
   execution counts and success rates, and finally the list of tests
   that were never executed by any matching job.

## Usage

Must be run with the current working directory set to the root of the
`test-infra` repository (so it can find `config/prow/config.yaml` and the
given job directory).

```
go run ./test-coverage -kubernetes-repo <path> [-e2e-suite e2e|e2e_node] [-job-dir <path>] [-age <duration>] [-job-filter <regexp>] [-workers <n>]
```

Flags:

* `-kubernetes-repo` (required): path to a `kubernetes/kubernetes`
  checkout, used to list the suite's tests via `--list-tests`.
* `-e2e-suite` (default `e2e_node`): which suite to analyze, `e2e` or
  `e2e_node`.
* `-job-dir` (default depends on `-e2e-suite`): directory of periodic job
  definitions to scan, relative to the test-infra repo root. Defaults to
  `config/jobs/kubernetes` for `e2e` and
  `config/jobs/kubernetes/sig-node` for `e2e_node`.
* `-age` (default `24h`): how far back to look for job runs.
* `-job-filter` (default `.*`): regular expression used to filter
  periodic job names.
* `-workers` (default `10`): number of concurrent workers used to find
  job runs and to download and analyze their JUnit results from GCS.
  Increasing it speeds up the tool considerably when many jobs or runs
  match, at the cost of more concurrent GCS requests.

## Examples

### `e2e_node`

```
$ go run ./test-coverage -kubernetes-repo ../kubernetes -e2e-suite e2e_node -job-filter '^ci-node-crio-dra' -age 6h
...
Found 2 periodic jobs: [ci-node-crio-dra ci-node-crio-dra-alpha-beta-features]
Finding runs of ci-node-crio-dra since 2026-07-08T05:35:25+02:00 ...
Found 1 runs of ci-node-crio-dra.
Finding runs of ci-node-crio-dra-alpha-beta-features since 2026-07-08T05:35:25+02:00 ...
Found 1 runs of ci-node-crio-dra-alpha-beta-features.
```

Truncated report output:

```
Test execution coverage report

Jobs analyzed (2), with number of runs found and average tests executed per run (see https://prow.k8s.io/job-history/gs/kubernetes-ci-logs/logs/<job name>):
  ci-node-crio-dra                      1  e2e_node  38.0
  ci-node-crio-dra-alpha-beta-features  1  e2e_node  38.0
  TOTAL                                 2            38.0

Executed tests (38 of 1217, 100% overall success rate):

[sig-node] [DRA] [Feature:DynamicResourceAllocation] [FeatureGate:DynamicResourceAllocation] Resource Health [FeatureGate:ResourceHealthStatus] must reuse one gRPC connection for service and health-monitoring calls [Beta] [Serial]: total 2 100.0%
    ci-node-crio-dra                      1  100.0%
    ci-node-crio-dra-alpha-beta-features  1  100.0%
[sig-node] [DRA] [Feature:DynamicResourceAllocation] [FeatureGate:DynamicResourceAllocation] Resource Health [FeatureGate:ResourceHealthStatus] should automatically reconnect both DRA and Health API after connection drop [Beta] [Serial]: total 2 100.0%
    ci-node-crio-dra                      1  100.0%
    ci-node-crio-dra-alpha-beta-features  1  100.0%
...

Tests never executed (1179 of 1217):

[sig-network] Networking Granular Checks: Pods should function for intra-pod communication: http [Conformance] [NodeConformance]
[sig-network] Networking Granular Checks: Pods should function for intra-pod communication: sctp [LinuxOnly] [Feature:SCTPConnectivity]
[sig-network] Networking Granular Checks: Pods should function for intra-pod communication: udp [Conformance] [NodeConformance]
...
```

### `e2e`

```
$ go run ./test-coverage -kubernetes-repo ../kubernetes -e2e-suite e2e -job-filter '^ci-kind-dra.*' -age 12h
...
Found 5 periodic jobs: [ci-kind-dra ci-kind-dra-all ci-kind-dra-n-1 ci-kind-dra-n-2 ci-kind-dra-n-3]
Finding runs of ci-kind-dra since 2026-07-08T01:23:53+02:00 ...
Finding runs of ci-kind-dra-n-3 since 2026-07-08T01:23:53+02:00 ...
Finding runs of ci-kind-dra-n-1 since 2026-07-08T01:23:53+02:00 ...
Finding runs of ci-kind-dra-n-2 since 2026-07-08T01:23:53+02:00 ...
Finding runs of ci-kind-dra-all since 2026-07-08T01:23:53+02:00 ...
Found 2 runs of ci-kind-dra.
Found 2 runs of ci-kind-dra-n-1.
Found 2 runs of ci-kind-dra-n-2.
Found 2 runs of ci-kind-dra-all.
Found 2 runs of ci-kind-dra-n-3.
```

Truncated report output:

```
Test execution coverage report

Jobs analyzed (5), with number of runs found and average tests executed per run (see https://prow.k8s.io/job-history/gs/kubernetes-ci-logs/logs/<job name>):
  ci-kind-dra      2   e2e  99.0
  ci-kind-dra-all  2   e2e  114.0
  ci-kind-dra-n-1  2   e2e  70.0
  ci-kind-dra-n-2  2   e2e  70.0
  ci-kind-dra-n-3  2   e2e  64.0
  TOTAL            10       83.4

Executed tests (88 of 7677, 100% overall success rate):

[sig-api-machinery] ResourceQuota should create a ResourceQuota and capture the life of a ResourceClaim [FeatureGate:DynamicResourceAllocation] [DRA]: total 4 100.0%
    ci-kind-dra      2  100.0%
    ci-kind-dra-all  2  100.0%
[sig-node] [DRA] CRUD Tests resource.k8s.io/v1 DeviceClass [Conformance]: total 4 100.0%
    ci-kind-dra      2  100.0%
    ci-kind-dra-all  2  100.0%
...

Tests never executed (7589 of 7677):

[sig-api-machinery] API Streaming (aka. WatchList) [FeatureGate:WatchList] reflector doesn't support receiving resources as Tables [Beta]
[sig-api-machinery] API Streaming (aka. WatchList) [FeatureGate:WatchList] reflector using standard List doesn't support receiving resources as Tables [Beta]
[sig-api-machinery] API Streaming (aka. WatchList) [FeatureGate:WatchList] server supports sending resources in Table format [Beta]
...
```

### `e2e`, showing different kinds of analyzed jobs

This example uses a filter that picks out three jobs that each illustrate a
different case handled by the "Jobs analyzed" table: a job that actually
runs the `e2e` suite (`ci-kind-dra`), a job that runs a different suite
(`ci-kubernetes-integration-master`, a gotestsum-based Go integration test
job detected as `many`, so only its single most recent run needs to be
checked before it is recognized as the wrong suite and skipped), and a job
that produces no JUnit output at all (`ci-kubernetes-build-fast`, a build
job). Since that last kind of job never allows the tool to conclude
anything about which suite it ran, *all* of its runs found within `-age`
have to be downloaded and checked, which is why it shows a much higher
"number of runs found" than the other two jobs despite matching no tests
at all:

```
$ go run ./test-coverage -kubernetes-repo ../kubernetes -e2e-suite e2e -job-filter '^ci-kind-dra$|^ci-kubernetes-integration-master$|^ci-kubernetes-build-fast$' -age 6h
...
Found 3 periodic jobs: [ci-kind-dra ci-kubernetes-build-fast ci-kubernetes-integration-master]
Finding runs of ci-kind-dra since 2026-07-08T10:51:18+02:00 ...
Finding runs of ci-kubernetes-integration-master since 2026-07-08T10:51:18+02:00 ...
Finding runs of ci-kubernetes-build-fast since 2026-07-08T10:51:18+02:00 ...
Found 1 runs of ci-kind-dra.
Found 6 runs of ci-kubernetes-integration-master.
Found 59 runs of ci-kubernetes-build-fast.
Ignoring ci-kubernetes-integration-master: ran suite "many" instead of "e2e".
Analyzed 1 runs of ci-kubernetes-integration-master.
Analyzed 1 runs of ci-kind-dra.
Analyzed 59 runs of ci-kubernetes-build-fast.
```

Truncated report output:

```
Test execution coverage report

Jobs analyzed (3), with number of runs found and average tests executed per run (see https://prow.k8s.io/job-history/gs/kubernetes-ci-logs/logs/<job name>):
  ci-kubernetes-build-fast          59  none  0.0
  ci-kubernetes-integration-master  1   many  0.0
  ci-kind-dra                       1   e2e   99.0
  TOTAL                             61        1.6
...
```
