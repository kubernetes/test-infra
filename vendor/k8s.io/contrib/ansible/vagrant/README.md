## Welcome to the Vagrant Deployer for Kubernetes Ansible

This deployer sets-up a Kubernetes cluster on Vagrant.

## Before You Start

Make sure you have [git installed](https://git-scm.com/book/en/v2/Getting-Started-Installing-Git) and clone the contrib repo:
```
git clone https://github.com/kubernetes/contrib.git
```

[Install Vagrant](https://www.vagrantup.com/downloads.html) if it's not currently installed on your system.

You will need a functioning [vagrant provider](https://www.vagrantup.com/docs/providers/). Currently supported providers are openstack, libvirt, and virtualbox. Vagrant comes with VirtualBox support by default. No matter what provider you choose, you need to install the OpenStack Vagrant plugin:

```
vagrant plugin install vagrant-openstack-provider --plugin-version ">= 0.6.1"
```

Vagrant uses Ansible to automate the Kubernetes deployment. Install Ansible (Mac OSX example):
```
sudo easy_install pip
sudo pip install ansible==2.0.0.2
```

Reference [Ansible installation](http://docs.ansible.com/ansible/intro_installation.html) for additional installation instructions.

The DNS kubernetes-addon requires python-netaddr. Install netaddr (Mac OSX example):

```
sudo pip install netaddr
```

Reference the [python-netaddr documentation](https://pythonhosted.org/netaddr/installation.html) for additional installation instructions.

### Fedora

When running the ``vagrant`` on Fedora (tested on F24) don't forget to install necessary dependencies:

```
dnf install -y ruby-devel gcc redhat-rpm-config
```

When provisioning VMs with libvirt provider, don't forget to install ``vagrant-libvirt``:

```
dnf install -y vagrant-libvirt
```

If you hit the ``Error while creating domain: Error saving the server: Call to virDomainDefineXML failed: invalid argument: could not find capabilities for domaintype=kvm `` error message, enable virtualization in BIOS [1], [2].

[1] https://github.com/vagrant-libvirt/vagrant-libvirt/issues/539  
[2] https://bugzilla.redhat.com/show_bug.cgi?id=1326561  

## Caveats

Vagrant (1.7.x) does not properly select a provider. You will need to manually specify the provider. Refer to the Provider Specific Information section for using the proper `vagrant up` command.

Vagrant prior version 1.8.0 doesn't write group variables into Ansible inventory file, which is required for using Core OS images.

## Usage

You can change some aspects of configuration using environment variables.
Note that these variables should be set for all vagrant commands invocations,
`vagrant up`, `vagrant provision`, `vagrant destroy`, etc.

### Configure number of nodes

If you export an env variable such as
```
export NUM_NODES=4
```

The system will create that number of nodes. Default is 2.

### Configure OS to use

You can specify which OS image to use on hosts:

```
export OS_IMAGE=centos7
```

By default CentOS 7 image is used.

Supported images:

* `centos7` (default) - CentOS 7 supported on OpenStack, VirtualBox, Libvirt providers.
* `coreos` - [CoreOS](https://coreos.com/) supported on VirtualBox provider.
* `fedora` - supported at least on Libvirt provider

### Start your cluster

If you are not running Vagrant 1.7.x or older, then change to the vagrant directory and `vagrant up`:

```
vagrant up
```


Vagrant up should complete with a successful Ansible playbook run:
```
....

PLAY RECAP *********************************************************************
kube-master                : ok=266  changed=78   unreachable=0    failed=0
kube-node-1                : ok=129  changed=39   unreachable=0    failed=0
kube-node-2                : ok=128  changed=39   unreachable=0    failed=0
```

Login to the Kubernetes master:
```
vagrant ssh kube-master
```

Verify the Kuberenetes cluster is up:
```
[vagrant@kube-master ~]$ kubectl cluster-info
Kubernetes master is running at http://localhost:8080
Elasticsearch is running at http://localhost:8080/api/v1/proxy/namespaces/kube-system/services/elasticsearch-logging
Heapster is running at http://localhost:8080/api/v1/proxy/namespaces/kube-system/services/heapster
Kibana is running at http://localhost:8080/api/v1/proxy/namespaces/kube-system/services/kibana-logging
KubeDNS is running at http://localhost:8080/api/v1/proxy/namespaces/kube-system/services/kube-dns
Grafana is running at http://localhost:8080/api/v1/proxy/namespaces/kube-system/services/monitoring-grafana
InfluxDB is running at http://localhost:8080/api/v1/proxy/namespaces/kube-system/services/monitoring-influxdb

[vagrant@kube-master ~]$ kubectl get nodes
NAME          LABELS                               STATUS    AGE
kube-node-1   kubernetes.io/hostname=kube-node-1   Ready     34m
kube-node-2   kubernetes.io/hostname=kube-node-2   Ready     34m
```

Make sure the STATUS shows Ready for each node. You are now ready to deploy Kubernetes resources. Try one of the [examples](https://github.com/kubernetes/kubernetes/tree/master/examples) from the Kubernetes project repo.

## Provider Specific Information
Vagrant tries to be intelligent and pick the first provider supported by your installation. If you want to specify a provider you can do so by running vagrant like so:
```
# virtualbox provider
vagrant up --provider=virtualbox

# openstack provider
vagrant up --provider=openstack

# libvirt provider
vagrant up --provider=libvirt
```

### OpenStack
Make sure you installed the openstack provider for vagrant.
```
vagrant plugin install vagrant-openstack-provider --plugin-version ">= 0.6.1"
```
NOTE This is a more up-to-date provider than the similar  `vagrant-openstack-plugin`.

Also note that current (required) versions of `vagrant-openstack-provider` are not compatible with ruby 2.2.
https://github.com/ggiamarchi/vagrant-openstack-provider/pull/237
So make sure you get at least version 0.6.1.

To use the vagrant openstack provider you will need
- Copy `openstack_config.yml.example` to `openstack_config.yml`
- Edit `openstack_config.yml` to include your relevant details.

###### Libvirt

The libvirt vagrant provider is non-deterministic when launching VMs. This is a problem as we need ansible to only run after all of the VMs are running. To solve this when using libvirt one must
do the following
```
vagrant up --no-provision
vagrant provision
```

### VirtualBox
Nothing special should be required for the VirtualBox provisioner. `vagrant up --provider virtualbox` should just work.


## Additional Information
If you just want to update the binaries on your systems (either pkgManager or localBuild) you can do so using the ansible binary-update tag. To do so with vagrant provision you would need to run
```
ANSIBLE_TAGS="binary-update" vagrant provision
```

### Running Ansible

After provisioning a cluster vith Vagrant you can run ansible in this directory for any additional provisioning -
`ansible.cfg` provides configuration that will allow Ansible to connect to managed hosts.

For example:

```
$ ansible -m setup kube-master
kube-master | SUCCESS => {
    "ansible_facts": {
        "ansible_all_ipv4_addresses": [
            "172.28.128.21",
            "10.0.2.15"
        ],
...
```

### Issues
File an issue [here](https://github.com/kubernetes/contrib/issues) if the Vagrant Deployer does not work for you or if you find a documentation bug. [Pull Requests](https://github.com/kubernetes/contrib/pulls) are always welcome :-) Please review the [contributing guidelines](https://github.com/kubernetes/kubernetes/blob/master/CONTRIBUTING.md) if you have not contributed in the past and feel free to ask questions on the [kubernetes-users Slack](http://slack.kubernetes.io) channel.

[![Analytics](https://kubernetes-site.appspot.com/UA-36037335-10/GitHub/contrib/ansible/vagrant/README.md?pixel)]()
