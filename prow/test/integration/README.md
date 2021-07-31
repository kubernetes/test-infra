
# Run Prow integration tests

## Run everything

This script would [setup](#setup) the environment, [run](#run-test) all the available integration tests, and then [cleans up](#cleanup) everything.

```bash
./prow/test/integration/integration-test.sh
```

## Setup

* [Setup a local registry](setup-local-registry.sh).
* Compile prow components, generate images, and push them to the registry.
* [Create a local cluster](setup-cluster.sh) using [kind](https://kind.sigs.k8s.io/).
* Wait for prow components to be ready.

```bash
./prow/test/integration/integration-test.sh setup
```

## Run test

After prow is installed on top of the local cluster, run the integration tests under [test](test) directory

Optional parameters:

* [--test_filter](https://docs.bazel.build/versions/main/command-line-reference.html#flag--test_filter) - Specifies a filter to forward to the test framework.
* [--cache_test_results](https://docs.bazel.build/versions/main/command-line-reference.html#flag--cache_test_results) - Whether to cache test results.

```bash
./prow/test/integration/integration-test.sh run [--test_filter=TestHook] [--cache_test_results=no]
```

## Cleanup

Delete the local cluster and the local registry.

```bash
./prow/test/integration/integration-test.sh teardown
```
