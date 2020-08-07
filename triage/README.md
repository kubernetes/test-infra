# ![Triage](logo.svg)

Triage identifies clusters of similar test failures across all jobs.

Use it here: https://go.k8s.io/triage


## Intro

Triage consists of two parts: a summarizer, which clusters together similar test failure messages,
and a web page which can be used to browse the results. The web page is a static HTML page which grabs
the results in JSON format, parses them, and displays them.


## Usage

Triage summarization is generally run via `update_summaries.sh`, which downloads the input files in
the correct format and passes them automatically to `triage`. (File formats are listed below.)
However, summarization can be run directly with the following flags:
- `builds`: a path to a JSON file containing build information
- `tests`: a space-delimited series of paths to file containing test informatiob
- `previous` (optional): a path to a previous output which can be used to maintain consistent cluster
  IDs
- `owners` (optional): a path to a file that maps SIGs to the labels they own (see [Methodology](#methodology));
  no longer used as labels are read straight from test names
- `output` (optional): the path to where the output should be written to; defaults to `./failure_data.json`
- `outputSlices` (optional): a pattern to be used when outputting slices, if desired (see
  [Methodology](#methodology)); e.g. `slices/failure_data_PREFIX.json`, where `PREFIX` will be replaced
  with some identifier

The web page can be accessed at https://go.k8s.io/triage with the following options:
- `Date`: defaults to "today"; note that all usages of "today" on the page refer to the currently set date
- `Show clusters for SIG`: filter results by the SIG assigned to the majority of the tests; allows multi-select
- `Include results from`: toggle between CI tests, PR tests, or both
- `Sort by`: basic sorting
- `Include filter`/`Exclude filter`: advanced regex filtering by field


## Go Packages

Package `berghelroach` contains a modified Levenshtein distance formula. Its only export is a `Dist()` function.  
Package `summarize` depends on package `berghelroach` and does the actual heavy lifting.


## Methodology

The entire process is orchestrated by `update_summaries.sh`, as follows:

1. Download all builds for the last 14 days from BigQuery.
1. Download all failed tests for the last 14 days from BigQuery.
1. Run `triage`:
   1. Load the downloaded files, and convert them into a format that Go can handle better (i.e. by
      parsing numbers).
   1. Group the builds by their build paths, and the test failures by their test names.
   1. Load previous results (if any) to aid in computation.
   1. Create a local clustering of the test failures from step 2. This splits each group of test
      failures into local clusters, i.e. groups of failures with similar failure texts. The mapping
      at this point is `Test Name => Local Cluster Text => Group of Test Failures`.
   1. Create a global clustering of the local clusters from the previous step, optionally using the
      previous results. This takes each local cluster and attempts to find clusters from other tests
      with similar cluster texts. If one is found, they are merged into a global cluster, with each
      test's failures remaining separate within the global cluster. The mapping at this point is
      `Global Cluster Text => Test Name => Group of Test Failures`.
   1. Transform the global clustering into a format that compresses better, and which is consumable
      by the web page.
   1. If a mapping of owners to owner prefixes (such as `sig-testing => [sig-testing]`) was provided
      as a flag, load it.
   1. Annotate each cluster with an owner, by parsing the test name or using the provided mapping
      from the previous step. This can be used to filter the clusters by SIG on the web page.
   1. Write the results to a JSON file.
   1. If the `output_slices` flag is set, create individual files ("slices") for each owner. Also,
      split the results into 256 slices based on the cluster IDs. Write the slices to JSON files.
1. Upload the results into Google Cloud Storage so they can be browsed via the web page.


## File Structure

Below are the file structures for the ingested and outputted files. `...` denotes a repetition of the
previous element.

### `builds` Flag
```json
[
   {
      "path": string,
      "started": int as string,
      "elapsed": int as string,
      "tests_run": int as string,
      "tests_failed": int as string,
      "result": string,
      "executor": string,
      "job": string,
      "number": int as string,
      "pr": string,
      "key": string
   },
   ...
]
```

### `tests` Flag
This is a newline-delimited list of JSON objects. **Note the lack of trailing comma between objects!**
```json
{
   "started": int as string,
   "build": string,
   "name": string,
   "failure_text": string
}
...
```

### `previous` Flag (same as Main Output)
```json
{
   "clustered": [
      {
         "key": string,
         "id": string,
         "text": string,
         "spans": [
            int,
            ...
         ],
         "tests": [
            {
               "name": string,
               "jobs": [
                  {
                     "name": string,
                     "builds": [
                        int,
                        ...
                     ]
                  },
                  ...
               ]
            },
            ...
         ],
         "owner": string,
      },
      ...
   ],
   "builds": {
      "jobs": {
         string: ([int, ...] OR {int as string: int, ...})  // See the description of the jobCollection type
      },
      "cols": {
         "started": [int, ...],
         "tests_failed": [int, ...],
         "elapsed": [int, ...],
         "tests_run": [int, ...],
         "result": [string, ...],
         "executor": [string, ...],
         "pr": [string, ...]
      },
      "job_paths": {
         string: string,
         ...
      },
   }
}
```

### `owners` Flag
```json
{
   string: [
      string,
      ...
   ],
   ...
}
```

### Main Output
See `previous` flag.

### Slice Output
See Main Output. This is only a subset of the main output.


## Updating JS dependencies for the web page

See: `package.json` + `bazel run @yarn//:yarn install`
