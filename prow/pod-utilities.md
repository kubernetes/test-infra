# Pod Utilities

Pod utilities are small, focused Go programs used by `plank` to decorate user-provided `PodSpec`s
in order to increase the ease of integration for new jobs into the entire CI infrastructure. The
utilities today wrap the execution of the test code to ensure that the tests run against correct
versions of the source code, that test commands run in the appropriate environment and that output
from the test (in the form of status, logs and artifacts) is correctly uploaded to the cloud.

These utilities are integrated into a test run by adding `InitContainer`s and sidecar `Container`s
to the user-provided `PodSpec`, as well as by overwriting the `Container` entrypoint for the test
`Container` provided by the user. The following utilities exist today:

 - [`clonerefs`](./cmd/clonerefs/README.md): clones source code under test
 - [`initupload`](./cmd/initupload/README.md): records the beginning of a test in cloud storage
   and reports the status of the clone operations
 - [`entrypoint`](./cmd/entrypoint/README.md): is injected into the test `Container`, wraps the
   test code to capture logs and exit status
 - [`sidecar`](./cmd/sidecar/README.md): runs alongside the test `Container`, uploads status, logs
   and test artifacts to cloud storage once the test is finished