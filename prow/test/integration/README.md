# Run Prow integration tests

## Run everything

Just run:

```bash
./prow/test/integration/integration-test.sh
```

This script [sets up](#setup) the environment, [runs](#run-test) all the available integration tests, and then [cleans up](#cleanup) everything.

## Setup

- [Set up a local registry](setup-local-registry.sh).
- Compile prow components, generate images, and push them to the local registry.
- [Create a local cluster](setup-cluster.sh) using [kind](https://kind.sigs.k8s.io/).
- Wait for prow components to be ready.

To skip tearing down the kind cluster after running the tests, set `SKIP_TEARDOWN=true` when invoking the integration test:

```bash
SKIP_TEARDOWN=true ./prow/test/integration/integration-test.sh
```

## Cleanup

If the local cluster exists, run the tests and then delete the local cluster and the local registry with:

```bash
SKIP_SETUP=true ./prow/test/integration/integration-test.sh
```

# Add new integration tests

## Add new components

(Assume the component to be added is named `most-awesome-component`)

- Add `most-awesome-component` at [`testimage-push`](https://github.com/kubernetes/test-infra/blob/f9fb6d28ebbcf77dc0b99d741b8df5f5d85c739e/prow/BUILD.bazel#L66) target, so that the component is pushed to `localhost:5001` registry
- Deploy `most-awesome-component` during integration test https://github.com/kubernetes/test-infra/blob/f9fb6d28ebbcf77dc0b99d741b8df5f5d85c739e/prow/test/integration/setup-cluster.sh#L33, and cleanup the component after integration test https://github.com/kubernetes/test-infra/blob/f9fb6d28ebbcf77dc0b99d741b8df5f5d85c739e/prow/test/integration/cleanup.sh#L23
- Add `most-awesome-component` deployment yaml at https://github.com/kubernetes/test-infra/tree/master/prow/test/integration/prow/cluster, so that the deployment works. Modifications involve:
  - `most-awesome-component_service.yaml` and `most-awesome-component_rbac.yaml` can be symlinks from https://github.com/kubernetes/test-infra/tree/master/config/prow/cluster.
  - `most-awesome-component_deployment.yaml` will at least requires changing image registry to `localhost:5001` like https://github.com/kubernetes/test-infra/blob/f9fb6d28ebbcf77dc0b99d741b8df5f5d85c739e/prow/test/integration/prow/cluster/hook_deployment.yaml#L41.
  - [If using github client] `github-endpoint` should be changed to `fakeghserver`, which was from https://github.com/kubernetes/test-infra/tree/master/prow/test/integration/fakeghserver.
- [If using github client] Existing fake github server only implemented partial github APIs, will need to add APIs that `most-awesome-component` uses at https://github.com/kubernetes/test-infra/tree/master/prow/test/integration/fakeghserver

## Add new tests

Tests are implemented in Go and are located under the [`test`](./test) directory.

- [If this is a new component] Create a file called `most-awesome-component_test.go`
- Add test in `most-awesome-component_test.go`
