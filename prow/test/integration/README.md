# Run Prow integration tests

## Run all integration tests

```bash
./prow/test/integration/integration-test.sh
```

## Run a specific integration test

```bash
./prow/test/integration/integration-test.sh -run=TestIWantToRun
```

## Cleanup

```bash
./prow/test/integration/teardown.sh -all
```

# Adding new integration tests

## New component

Assume we want to add `most-awesome-component` (source code in `prow/cmd/most-awesome-component`).

1. Add `most-awesome-component` to the `PROW_COMPONENTS`, `PROW_IMAGES`, and
   `PROW_IMAGES_TO_COMPONENTS` variables in [lib.sh](lib.sh).

   - Add the line `most-awesome-component` to `PROW_COMPONENTS`.
   - Add the line `[most-awesome-component]=prow/cmd/most-awesome-component` to `PROW_IMAGES`.
   - Add the line `[most-awesome-component]=most-awesome-component` to `PROW_IMAGES_TO_COMPONENTS`.
   - Explanation: `PROW_COMPONENTS` lists which components are deployed into the
     cluster, `PROW_IMAGES` describes where the source code is located for each
     component (in order to build them), and finally `PROW_IMAGES_TO_COMPONENTS`
     defines the relationship between the first two variables (so that the test
     framework knows what to redeploy depending on what image has changed). As an
     example, the `deck` and `deck-tenanted` components (in `PROW_COMPONENTS`)
     both use the `deck` image (defined in `PROW_IMAGES_TO_COMPONENTS`), so they
     are both redeployed every time you change something in `prow/cmd/deck`
     (defined in `PROW_IMAGES`).

2. Set up Kubernetes Deployment and Service configurations inside the
   [configuration folder][config/prow/cluster] for your new component. This
   way the test cluster will pick it up when it [deploys Prow
   components](setup-prow-components.sh).

   - If you want to deploy an existing Prow component used in production (i.e.,
     https://prow.k8s.io), you can reuse (symlink) the configurations used in
     production. See the examples in [configuration folder][config/prow/cluster].

   - Remember to use `localhost:5001/most-awesome-component` for the `image: ...`
     field in the Kubernetes configurations to make the test cluster use the
     freshly-built image.

## New tests

Tests are written under the [`test`](test) directory. They are named with the
pattern `<COMPONENT>_test.go*`. Continuing the example above, you would add new
tests in `most-awesome-component_test.go`

## Check that your new test is working

1. Add or edit new tests (e.g., `func TestMostAwesomeComponent(t *testing.T) {...}`) in `most-awesome-component_test.go`.
2. Run `./prow/test/integration/integration-test.sh -run=TestMostAwesomeComponent` to bring up the test cluster and to only test
   your new test named `TestMostAwesomeComponent`.
3. If you need to make changes to `most-awesome-component_test.go` (and not the
   component itself), run `./prow/test/integration/integration-test.sh -run=TestMostAwesomeComponent -no-setup`. The `-no-setup` will ensure that
   the test framework avoid redeploying the test cluster.
   - If you **do** need to make changes to the Prow component, run
     `./prow/test/integration/integration-test.sh -run=TestMostAwesomeComponent -build=most-awesome-component` so that `prow/cmd/most-awesome-component` is
     recompiled and redeployed into the cluster before running
     `TestMostAwesomeComponent`.

If Step 2 succeeds and there is nothing more to do, you're done! If not (and
your tests still need some tweaking), repeat steps 1 and 3 as needed.

# How it works

In short, the [integration-test.sh](integration-test.sh) script creates a
[KIND](https://kind.sigs.k8s.io/) Kubernetes cluster, runs all available
integration tests, and finally deletes the cluster.

Recall that Prow is a collection of services (Prow components) that can be
deployed into a Kubernetes cluster. KIND provides an environment where we can
deploy certain Prow components, and then from the integration tests we can
create a Kubernetes Client to talk to this deployment of Prow.

Note that the integration tests do not test all components (we need to fix
this). [The PROW_COMPONENTS variable](lib.sh) is a list of components currently
tested. These components are compiled and deployed into the test cluster on
every invocation of [integration-test.sh](integration-test.sh).

Each tested component needs a Kubernetes configuration so that KIND understands
how to deploy it into the cluster, but that's about it (more on this below). The
main thing to keep in mind is that the integration tests must be hermetic and
reproducible. For this reason, all components that are tested must be configured
so that they do not attempt to reach endpoints that are outside of the cluster.
For example, this is why some Prow components have a `-github-endpoint=...` flag
that you can use --- this way these components can be instructed to talk to the
`fakeghserver` deployed inside the cluster instead of trying to talk to GitHub.

# Code layout

```
.
├── cmd # Binaries for fake services deployed into the test cluster along with actual Prow components.
│   ├── fakegerritserver # Fake Gerrit.
│   ├── fakeghserver # Fake GitHub.
│   └── fakegitserver # Fake low-level Git server. Can theoretically act as the backend for fakeghserver or fakegerritserver.
├── config # Kubernetes configuration files.
│   └── prow # Prow configuration for the test cluster.
│       ├── cluster # KIND test cluster configurations.
│       └── jobs # Static Prow jobs. Some tests use these definitions to run Prow jobs inside the test cluster.
├── internal
│   └── fakegitserver
└── test # The actual integration tests to run.
    └── testdata # Test data.
```

[test-cluster-images]: ./prow/.prow-images.yaml
