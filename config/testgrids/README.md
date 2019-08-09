# Testgrid Configurations

This readme covers information specific to testgrid.k8s.io and this repository.
See Testgrid's [config.md](../../testgrid/config.md) for more information Testgrid config files.

## Adding a Prow Job to Testgrid

Prow Jobs in this repository only need to be [annotated](/testgrid/config.md#prow-job-configuration);
no changes are necessary here unless you are adding a brand new dashboard.

## Adding or Changing a Configuration

Any file put in this directory or a subdirectory will be picked up by [testgrid.k8s.io](https://testgrid.k8s.io).

## Testing

Run `bazel test //testgrid/...` to ensure the configuration is valid.

This finds common problems such as malformed yaml, a tab referring to a
non-existent test group, a test group never appearing on any tab, etc.

Run `bazel test //...` for slightly more advanced testing, such as ensuring that
every job in our CI system appears somewhere in testgrid, etc.