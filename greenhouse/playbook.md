# Greenhouse Playbook

This is the playbook for GreenHouse. See also [the playbook index][playbooks].

TDLR: Greenhouse is a [bazel] [remote build cache][remote-build-cache].

The [OWNERS][OWNERS] are a potential point of contact for more info.

For in depth details about the project see the [README][README].

## General Debugging

Greenhouse runs as a Kubernetes deployment.

For the [Kubernetes Project's Prow Deployment][prow-k8s-io] the exact spec is in
[deployment.yaml], and the deployment is in the "build cluster".

### Logs

First configure your local environment to point to the cluster hosting 
greenhouse. <!--TODO: link to prow info for doing this on our deployment-->

The greenhouse pods should have the label `app=greenhouse`, you can view
the logs with `kubectl logs -l=app=greenhouse`.

The logs may also be stored in [Stackdriver] / the host cluster's logging
integration(s).

### Monitoring

The Kubernetes project's deployment has [some monitoring on velodrome][velodrome].
You may want to check if there has been a sudden change in stats.

Note that periodicly freeing disk space is expected, and will be highly variable
with the load from our build workloads. 

Writing half a terabyte of cache entries and reaching the eviction threshold
in just 3 to 4 hours is not unusual under load as of 9/10/2019.

## Options

The following well-known options are available for dealing with greenhouse
service issues.

### Rolling Back

If you are running greenhouse without using the config in this repo 
(and you likely are if you are not looking at prow.k8s.io ...) you will need
to roll back the specific deployment mechanism used in that deployment.

For prow.k8s.io kf you think that Greenhouse is broken in some way the easiest 
way to roll it back is to check out this repo to a previous commit deploy it 
from that commit.

Deployment details are covered in the [README].

### Cutting Access To The Cache

Cache users must explicitly configure bazel to use the cache and will fall
back to non-cached builds if the cache cannot be reached.

To force falling back, you can simply delete the `bazel-cache` service.

`kubectl delete service bazel-cache -l=app=greenhouse`

Eventually once we've resolved whatever issue necessitates this, you should
reinstate the service, which is defined in [service.yaml].

### Wiping The Cache Contents

Firstly: you should not do this! This is only necessary if there is a bad
bug in bazel related to caching for some reason.

If this does become the case and you are confident you can do this fairly
trivially. However this will be mildly disruptive due to trying to delete 
files that may be currently being served. 

You should only do this if you really think that somehow bazel
has bad state in the cache.

You should also consider removing the greenhouse service instead, jobs
will fall back to non-cached builds if the cache cannot be reached.

If you do decide to do this, this is how:

- Find the pod name with `kubectl get po -l=app=greenhouse`
- Obtain a shell the pod with `kubectl exec -it <greenhouse-pod-name> /bin/sh`
- The data directory should be at `/data` for our deployment.
  - Verify this by inspecting `kubectl describe po -l=app=greenhouse`
- Once you are sure that you know where the data is stored, you can simply run
`cd /data && rm -rf ./*` from the `kubectl exec` shell you created above.

## Known Issues

Greenhouse has a relatively clean track record for approximately two years now.

I've probably just jinxed it.

There is, however at least one known issue with Bazel caching in general that
may affect Greenhouse users at some point.

### Host Tool Tracking Is Limited

Bazel does not properly track toolchains on the host (like C++ compilers).

This issue may occur with bazel's machine local cache (`$HOME/.cache/bazel/...`)
on your development machine.

To avoid this problem with our cache we ask that users of greenhouse use some 
additional tooling we built to ensure that the cache is used in a way that
includes a key hashed from the known host toolchains in use.

You can read more about how we do this in the [README], and the upstream issue 
[bazel#4558].

It is possible that in the future one of our cache users will depend on some
"host" toolchain that bazel does not track as an input, causing issues when
versions of the tool are switched and produce incompatible outputs.

This may be difficult to diagnose if you are not familiar with Bazel's output.
Consider asking for help from someone familiar with Bazel if you suspect this
issue.

<!--URLS-->
[OWNERS]: ./OWNERS 
[README]: ./README.md 
[playbooks]: ./../docs/playbooks.md
<!--Additional URLS-->
[bazel]: https://bazel.build/
[remote-build-cache]: https://docs.bazel.build/versions/master/remote-caching.html
[deployment.yaml]: ./deployment.yaml
[service.yaml]: ./deployment.yaml
[prow-k8s-io]: https://prow.k8s.io
[bazel#4558]: https://github.com/bazelbuild/bazel/issues/4558
[velodrome]: http://velodrome.k8s.io/dashboard/db/bazel-cache?refresh=1m&orgId=1
[Stackdriver]: https://cloud.google.com/stackdriver/
