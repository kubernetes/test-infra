# Run Prow integration tests

## Run everything

Just run:

```bash
./prow/test/integration/integration-test.sh
```

This script creates a [KIND](https://kind.sigs.k8s.io/) Kubernetes cluster, runs all available integration tests, and finally deletes the cluster.

# How it works

Recall that Prow is a collection of services (Prow components) that can be
deployed into a Kubernetes cluster. KIND provides an environment where we can
deploy certain Prow components, and then from the integration tests we can
create a Kubernetes Client to talk to this deployment of Prow.

Note that the integration tests do not test all components (we need to fix
this). [Here][test-cluster-images] is a list of components currently tested.
These components are compiled and deployed into the test cluster on every
invocation of [integration-test.sh](integration-test.sh).

Each tested component needs a Kubernetes configuration so that KIND understands
how to deploy it into the cluster, but that's about it (more on this below). The
main thing to keep in mind is that the integration tests must be hermetic and
reproducible. For this reason, all components that are tested must be configured
so that they do not attempt to reach endpoints that are outside of the cluster.
For example, this is why some Prow components have a `-github-endpoint=...` flag
that you can use --- this way these components can be instructed to talk to the
`fakeghserver` deployed inside the cluster instead of trying to talk to GitHub.

## Setup

The setup phase involves:

1. [creating a local Docker registry](setup-local-registry.sh) inside the cluster, and
2. [deploying Prow components](setup-cluster.sh) into the cluster.

To skip tearing down the kind cluster after running the tests, set `SKIP_TEARDOWN=true` when invoking the integration test:

```bash
SKIP_TEARDOWN=true ./prow/test/integration/integration-test.sh
```

## Cleanup

If the test cluster already exists, run the tests and then delete the cluster
with:

```bash
SKIP_SETUP=true ./prow/test/integration/integration-test.sh
```

# Adding new integration tests

## New component

Assume we want to add `most-awesome-component`.

- Add `most-awesome-component` to the [list of binaries built by the integration
  tests][test-cluster-images], so that the component is pushed to the local
  Docker registry (`localhost:5001`) inside the test cluster.
- Make the test setup scripts aware of your new component by adding it to the
  `PROW_COMPONENTS` variable in [lib.sh](lib.sh).
- Set up Kubernetes Deployment and Service configurations inside the
  [configuration folder][./prow/cluster] for your new component. This way the
  test cluster will pick it up during the [cluster setup
  phase](setup-cluster.sh).
  - If you want to deploy an existing Prow component used in production (i.e.,
    https://prow.k8s.io), you can reuse (symlink) the configurations used in
    production. See the examples in [configuration folder][./prow/cluster].
  - Remember to use `localhost:5001/most-awesome-component` for the `image: ...`
    field in the Kubernetes configurations to make the test cluster use the
    freshly-built image.

## New tests

Tests are written under the [`test`](test) directory. They are named with the
pattern `<COMPONENT>_test.go*`. Continuing the example above, you would add new
tests in `most-awesome-component_test.go`

## Running specific tests

1. Run `SKIP_TEARDOWN=true ./prow/test/integration/integration-test.sh` to bring up the test cluster.
2. Add or edit new tests (e.g., `func TestMostAwesomeComponent(t *testing.T) {...}`) in `most-awesome-component_test.go`.
3. Run the test you are interested in against the test cluster. E.g., run `go test ./prow/test/integration/test -v --run-integration-test --run=TestMostAwesomeComponent`. This way you don't have to run all
   integration tests every time you make a change.

Repeat steps 2 and 3 as needed.

If you end up making changes to any of the [services defined in the KIND
cluster][test-cluster-images], you have to recompile and redeploy those
services back into the cluster. The simplest way to do this is:

```bash
# Delete existing (deprecated) KIND cluster.
./prow/test/integration/cleanup.sh
SKIP_TEARDOWN=true ./prow/test/integration/integration-test.sh
```

[test-cluster-images]: ./prow/.prow-images.yaml

# Code layout

```
.
├── cmd # Binaries for fake services deployed into the test cluster along with actual Prow components.
│   ├── fakegerritserver # Fake Gerrit.
│   ├── fakeghserver # Fake GitHub.
│   └── fakegitserver # Fake low-level Git server. Can theoretically act as the backend for fakeghserver or fakegerritserver.
├── internal # Library code used by the fakes and the integration tests in test/.
│   └── fakegitserver
├── prow # Prow configuration for the test cluster.
│   ├── cluster # KIND test cluster configurations.
│   └── jobs # Static Prow jobs. Some tests use these definitions to run Prow jobs inside the test cluster.
└── test # The actual integration tests.
    └── testdata # Test data.
```
