# Kubernetes Ansible

This playbook and set of roles set up a Kubernetes cluster onto machines. They
can be real hardware, VMs, things in a public cloud, etc. Anything that you can connect to via SSH.

## Before starting

* Record the IP address/hostname of which machine you want to be your master (only support a single master)
* Record the IP address/hostname of the machine you want to be your etcd server (often same as master, only one)
* Record the IP addresses/hostname of the machines you want to be your nodes. (the master can also be a node)
* Make sure your ansible running machine has ansible 1.9 and python-netaddr installed.

## Setup

### Configure inventory

Add the system information gathered above into a file called `inventory`,
or create a new one for the cluster.
Place the `inventory` file into the `./inventory` directory.

For example:

```sh
[masters]
kube-master-test.example.com

[etcd:children]
masters

[nodes]
kube-minion-test-[1:2].example.com
```

### Configure Cluster options

Look through all of the options in `inventory/group_vars/all.yml` and
set the variables to reflect your needs. The options are described there
in full detail.

#### Securing etcd

If you wish to use TLS certificates for your etcd cluster you have to specify TLS keypairs and https for `etcd_url_scheme`/`etcd_peer_url_scheme`. This will enable encrypted communication, but will not authenticate the clients certificate validity. To prevent unauthorized access to your etcd cluster, please set `etcd_client_cert_auth`/`etcd_peer_client_cert_auth` correspondingly to true.

## Running the playbook

After going through the setup, run the `deploy-cluster.sh` script from within the `scripts` directory:

`$ cd scripts/ && ./deploy-cluster.sh`

You may override the inventory file by running:

`INVENTORY=myinventory ./deploy-cluster.sh`

In general this will work on very recent Fedora, rawhide or F21.  Future work to
support RHEL7, CentOS, and possible other distros should be forthcoming.

### Targeted runs

You can just setup certain parts instead of doing it all.

#### Etcd

`$ ./deploy-cluster.sh --tags=etcd`

#### Kubernetes master

`$ ./deploy-cluster.sh --tags=masters`

#### Kubernetes nodes

`$ ./deploy-cluster.sh --tags=nodes`

### Addons

By default, the Ansible playbook deploys Kubernetes addons as well. Addons consist of:

* DNS (skydns)
* cluster monitoring (Grafana, Heapster, InfluxDB)
* cluster logging (Kibana, ElasticSearch)
* Kubernetes dashboard
* Kubernetes dash

In order to skip addons deployment, run

`$ ./deploy-cluster.sh --skip-tags=addons`

In order to run addons deployment only (requires kubernetes master already deployed), run

`$ ./deploy-cluster.sh --tags=addons` or `$ ./deploy-addons.sh`

### Component sources

Each component can be installed from various sources. For instance:

* distribution packages
* github release
* kubernetes built from local source codes

By default, every component (etcd, docker, kubernetes, etc.) is installed via distribution package manager.
Currently, the following component types are supported:

* `etcd_source_type`: for `etcd` role
* `flannel_source_type`: for `flannel` role
* `kube_source_type`: for `master` and `node` roles
* `source_type`: for other roles (and components)

To see a full list of available types, see corresponding role's default variables.

#### Kubernetes source type

Available types (see `kube_source_type` under `roles/kubernetes/defaults/main.yml`):

* `packageManager`
* `localBuild`
* `github-release`
* `distribution-rpm`

In case of a package manager, the `kube-apiserver` binary is shipped with `cap_net_bind_service=ep` capability set.
The capability allows the apiserver to listen on `443` port.
In a case of `localBuild` and `github-release`, the capability is not set.
In order for apiserver to listen on a secure port, change the port (see `kube_master_api_port` under `roles/kubernetes/defaults/main.yml`). For instance to listen on `6443`.

In order to apply the `distribution-rpm` type, location of an rpm must be specified.
See `kube_rpm_url_base` and `kube_rpm_url_sufix` variables under `roles/kubernetes/defaults/main.yml`.

### Network Service

By changing the `networking` variable in the `inventory/group_vars/all.yml` file, you can choose the network-service to use.  The default is flannel.

`$ ./deploy-cluster.sh --tags=network-service-install`

### Troubleshooting

* When updating flannel to version `0.5.5-7` or higher on Fedora, the `/etc/sysconfig/flannel` configuration file (if changed) must be updated to reflect renamed systemd environment variables.

[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/contrib/ansible/README.md?pixel)]()
