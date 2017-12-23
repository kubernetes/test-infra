# Kubernetes One Million Requests Per Second Demo

## Prerequisites
### Kubernetes version 1.1 or newer
This demo requires a Kubernetes cluster at version 1.1 or newer as it uses iptables proxy support that was
released in Kubernetes 1.1.

You can see what version you are running by running:

```shell
kubectl version
```

_Try it on a 1.0 cluster at your own risk._

### Create a sufficiently sized cluster
To scale to one million QPS you need a number of cores.  A 200 node cluster of n1-standard-4
VMs in [Google Container Engine](https://cloud.google.com/container-engine/) (or equivalent) (800
cores) should be sufficient.

Alternately, you can still run the demo at smaller scales, you just might not be able to hit one
million requests / second.

### Clone the demo repository

Then you need to clone the repository:
```shell
git clone https://github.com/kubernetes/contrib
cd contrib/scale-demo
```

### Activate IPTables proxying (1.1.x only)

Finally, if you are using a 1.1.x cluster, you will need to activate IPTables based proxying
(IPTable proxying is on by default in 1.2.x):
```shell
for node in $(kubectl get nodes -o name | cut -f2 -d/); do
    # switch to iptables mode
    kubectl annotate node $node net.beta.kubernetes.io/proxy-mode=iptables;
    # restart the proxies
    # This command is specific to GCE/GKE, other platforms may need different SSH commands
    gcloud compute ssh --zone=us-central1-b $node \
    	   --command="sudo /etc/init.d/kube-proxy restart";
done
```

## Set up the load test
In the `scale-nginx` directory, run the bootstrap script:

```shell
./bootstrap-demo.sh
```

This will create an initial nginx server, a single loadbot and an aggregator to aggregate data.
It will also create some [Services](http://kubernetes.io/v1.1/docs/user-guide/services.html) to
link the pieces together.

## Startup the UI
We'll use `kubectl proxy` to launch the UI:

```shell
kubectl proxy --www=${PWD}/www/
```

Then visit [http://localhost:8001/static/index.html](http://localhost:8001/static/index.html).

## Scale up nginx
First scale up nginx to handle approximately the load you expect to see.  In my experiments a 1 core
nginx server could handle approximately 10 thousand requests per second.  So to handle one million
we will scale up to 100 replicas.

```shell
kubectl scale rc nginx --replicas=100
```

## Scale up the loadbots
Each loadbot applies one thousands requests per second, so to get to one million requests per second
run one thousand loadbots.

```shell
kubectl scale rc vegeta --replicas=1000
```

## Rolling update
Once you have reached one million requests per second, you can perform a rolling update:

```shell
kubectl rolling-update nginx --image=gcr.io/google_containers/nginx-scale:0.3
```

The rolling update takes quite a while to complete.  If you want to run it faster (though slightly
more risky, use the `--update-period=1s` to decrease the time between container updates to one
second.

## Teardown
Once you are happy, tear down the load infrastructure with:

```shell
./teardown-demo.sh
```


