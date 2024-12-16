# config/tests/jobs

This directory contains tests for the jobs deployed on [prow.k8s.io]

These tests enforce a number of project-specific conventions.

To run via bazel: `bazel test //config/tests/jobs/...`

To run via go: `go test ./...`

If tests fail, re-run with the `UPDATE_FIXTURE_DATA=true` env variable
or use `make update-config-fixture`, then
include the modified files in the PR which updates the jobs.

[prow.k8s.io]: https://prow.k8s.io
