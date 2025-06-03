# CRI-O test infra jobs

**For any modifications to this directory, please ping
[@kubernetes/sig-node-cri-o-test-maintainers](https://github.com/orgs/kubernetes/teams/sig-node-cri-o-test-maintainers)
on the related issue or pull request.**

All jobs maintained within this directory are part of the `sig-node-cri-o`
testgrid dashboard: https://testgrid.k8s.io/sig-node-cri-o

---

This directory contains all CRI-O related test infra job ignition files. If you
want to change, add or remove any of those `*.ign` files, then please modify the
main configuration in [`./templates/generate`](./templates/generate), which
defines an associative array `CONFIGURATIONS` which defines each ignition file.

For example the configuration:

```bash
    ["crio_cgroupv1"]="root cgroupv1"
```

Will generate the [`crio_cgroupv1.ign`](./crio_cgroupv1.ign) configuration
containing the following base configurations in order:

1. [root.yaml](./templates/base/root.yaml)
1. [cgroupv1.yaml](./templates/base/cgroupv1.yaml)

When running `make` within this directory, an intermediate
[`./templates/crio_cgroupv1.yaml`](./templates/crio_cgroupv1.yaml)
[butane](https://coreos.github.io/butane) configuration will be generated which
then gets transformed into the resulting ignition file
[`crio_cgroupv1.ign`](./crio_cgroupv1.ign).
The ignition file will be then referenced from image configurations like
[`./latest/image-config-cgroupv1.yaml`](./latest/image-config-cgroupv1.yaml).

This means modifying, adding or removing jobs should always result in running
`make` as well as committing all changes into this repository.

If you want to test a ignition config in Google Cloud, ensure that you have
access to the VM by providing the SSH key for the user `core`, for example by
modifying `root.yaml`:

```yaml
passwd:
  users:
    - name: core
      ssh_authorized_keys:
        - ssh-rsa AAA…
```

Then spawn the instance via:

```sh
gcloud compute instances create \
    --zone europe-west1-b \
    --metadata-from-file user-data=/path/to/crio_cgroupv1.ign \
    --image-project fedora-coreos-cloud \
    --image-family fedora-coreos-stable my-instance
```

Accessing the virtual machine should be now possible by using the external IP of
the instance.

# Change CRI-O versions

To change the version of CRI-O being used for a single ignition file, just copy
[env.yaml](./templates/base/env.yaml) and adapt
[`./templates/generate`](./templates/generate) accordingly.

Make sure the specified cri-o version is uploaded to
`https://storage.googleapis.com/cri-o/artifacts/cri-o.amd64.{{ CRIO_COMMIT }}.tar.gz`,
otherwise the tests should fail.

You can test the cri-o version change by changing [env-canary.yaml](./templates/base/env-canary.yaml)
and run `pull-kubernetes-node-crio-cgrpv2-e2e-canary`.
