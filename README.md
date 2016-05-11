# Kubernetes Test Infrastructure

[![Build Status](https://travis-ci.org/kubernetes/test-infra.svg?branch=master)](https://travis-ci.org/kubernetes/test-infra)

## Test Results History

The [Kubernetes test history
dashboard](http://storage.googleapis.com/kubernetes-test-history/static/index.html)
contains e2e test results from the last 24 hours.  The test history is
implemented by two components in this repository:

1. `jenkins/test-history/` - scripts that periodically gather results from e2e
   test jobs and generate the above status page.
2. `gubernator/` - parses and presents the error message of a failed scenario.
   The test-history page links failed scenarios to the output of gubernator.
