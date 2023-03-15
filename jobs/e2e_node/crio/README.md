# CRI-O test infra jobs

This directory contains all CRI-O related test infra job ignition files. If you
want to change, add or remove any of those `*.ign` files, then please modify the
main configuration in [`./templates/generate`](./templates/generate), which
defines an associative array `CONFIGURATIONS` which defines each ignition file.

For example the configuration:

```bash
    ["crio"]="root cgroups-v1 dbus-tools-install crio-install"
```

Will generate the [`crio.ign`](./crio.ign) configuration containing the
following base configurations in order:

1. [root.yaml](./templates/base/root.yaml)
1. [cgroups-v1.yaml](./templates/base/cgroups-v1.yaml)
1. [dbus-tools-install.yaml](./templates/base/dbus-tools-install.yaml)
1. [crio-install.yaml](./templates/base/crio-install.yaml)

When running `make` within this directory, an intermediate
[`./templates/crio.yaml`](./templates/crio.yaml)
[butane](https://coreos.github.io/butane) configuration will be generated which
then gets transformed into the resulting ignition file [`crio.ign`](./crio.ign).
The ignition file will be then referenced from image configurations like
[`./latest/image-config-cgrpv1.yaml`](./latest/image-config-cgrpv1.yaml).

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
    --metadata-from-file user-data=/path/to/crio.ign \
    --image-project fedora-coreos-cloud \
    --image-family fedora-coreos-stable my-instance
```

Accessing the virtual machine should be now possible by using the external IP of
the instance.
