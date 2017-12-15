This filesystem branch contains a very simple example CNI plugin.
This plugin invokes `docker network` commands to connect or disconnect
a container.  In particular, this plugin's "add container to network"
operation connects the container to a Docker network that is
identified in the config file.  This plugin works with any existing
Docker network, regardless of which libnetwork driver/plugin made it.

To meet all of the functional requirements of
http://kubernetes.io/docs/admin/networking/#kubernetes-model some
additional static configuration is required, which you must do
yourself.

This plugin is written in bash so that is widely understandable and
does not require any building.


## Pre-requisites

You must have Kubernetes, Docker, bash, and [jq]
(https://stedolan.github.io/jq/) installed on each Kubernetes worker
machine ("node").


The Docker name of a Docker network must appear as the value of the
`name` field in `c2d.conf`.  This network must already exist before
you configure Kubernetes to use this CNI plugin.

For example, you might use the Kuryr libnetwork driver (after
installing and configuring it) to make a multi-host Docker network
with the following command.

```
docker network create -d kuryr --ipam-driver=kuryr --subnet=172.19.0.0/16 --gateway=172.19.0.1 mynet
```

For a trivial example, you might install Kubernetes configured to use
Flannel in the usual way --- and then modify the Kubernetes
configuration to use this CNI plugin to connect to the Docker network
named "bridge" (which is the one used with Flannel).  This
modification adds no functionality to your cluster, but it exercise
this CNI plugin.  Make sure you edit `c2d.conf` to change the network
name to `bridge`.

For an almost-example, you might try Docker's "overlay" driver to make
a multi-host Docker network with the following command.

```
docker network create -d overlay --subnet=172.19.0.0/16 --gateway=172.19.0.1 mynet
```

However, with Docker's "overlay" driver there is no way to add the
required additional connectivity from hosts to containers.  So do not
do this.

For another almost-example, you might try Calico's libnetwork driver,
with commands like the following.

```
calicoctl pool add 172.19.0.0/16 --nat-outgoing
calicoctl pool remove 192.168.0.0/16
docker network create --driver calico --ipam-driver calico mynet
```

However, Calico's libnetwork driver gives the name `cali0` to the
network interface inside the container --- which conflicts with
Kubernetes' requirement that this network interface be named `eth0`.
So do not do this.  Perhaps [eventually]
(https://github.com/appc/cni/issues/113) CNI will allow the network
interface name to be returned, which would enable Kubernetes to cope
with variant names.


## Installation and Configuration

This plugin has one config file and two executables.  Put the
executables in `/opt/cni/bin/`.  Put the config file, `c2d.conf`, in a
directory of your choice; `/etc/cni/net.d/` is the usual choice.

There are two configuration settings that must be made on each
kubelet.  The following expresses those settings as command line
arguments, assuming that `/etc/cni/net.d/` is the directory where you
put the config file.

```
--network-plugin=cni --network-plugin-dir=/etc/cni/net.d
```

If the config file has a field named `debug` with the value `true`
then each invocation of the plugin will produce some debugging output
in `/tmp/`.

A multi-host docker network does not necessarily meet the requirement
(seen in http://kubernetes.io/docs/admin/networking/#kubernetes-model)
that hosts and containers can open connections to each other.
However, you can typically enable this with a bit of static
configuration.  The particulars of this depend on the Docker network
you choose; this simple plugin does not attempt to discern and effect
that static configuration --- it is up to you.

For example, consider the case in which the multi-host Docker network
was created by the Kuryr libnetwork driver making a Neutron "tenant
network".  To complete the required connectivity, you might begin with
connecting that tenant network to a Neutron router with a command like
the following --- in which `f3e6fc55-c26e-4f05-bcb1-b84dc40a4233` is
the Neutron UUID of the subnet of the tenant network in question and
`router1` is the name of the Neutron router.

```
neutron router-interface-add router1 subnet=f3e6fc55-c26e-4f05-bcb1-b84dc40a4233
```

That is probably not all you will have to do.  The remainder depends
on details of your Neutron configuration.  There are two remaining
things to accomplish: getting service requests routed to something(s)
that serve the service cluster IP addresses, and getting replies
routed back to the client pods.  The first can be accomplished with
more or less complicated IP routing rules in the Neutron router(s)
involved.  For the routes from server back to client, covering all the
possibilities is beyond the scope of this simple example.  However, in
the easiest cases all you need to do is add a route in each host's
main network namespace, telling it how to route to the tenant network.
Following is an example command for that, which assumes that
`10.9.8.7` is an IP address that the host can use to reach the
relevant Neutron router and `172.19.0.0/16` is the tenant subnet.

```
ip route add 172.19.0.0/16 via 10.9.8.7
```
