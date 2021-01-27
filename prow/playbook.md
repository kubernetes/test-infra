# Prow Playbook

This is the playbook for Prow. See also [the playbook index][playbooks].

TDLR: Prow is a set of CI services.

The [OWNERS][OWNERS] are a potential point of contact for more info.

For in depth details about the project see the [README][README].

## General Debugging

Prow runs as a set of Kubernetes deployments.

For the [Kubernetes Project's Prow Deployment][prow-k8s-io] the exact spec is in
[cluster], and the deployment is in the "prow services cluster".

### Logs

If you are a googler checking prow.k8s.io, you may open `go/prow-debug` in your
browser. If you are not a googler but have access to this prow, you can
open [Stackdriver] logs in the `k8s-prow` GCP projects.

Other prow deployments may have their own logging stack.

### Monitoring

TODO ...

## Options

The following well-known options are available for dealing with prow
service issues.

### Rolling Back

For prow.k8s.io you can simply use `experiment/revert-bump.sh` to roll back
to the last checked in deployment version.

If prow is at least somewhat healthy, filing and merging PR from this will 
result in the rolled back version being deployed.

If not, you may need to manually run `bazel run //config/prow/cluster:production.apply`.


## Known Issues


### Something TODO

TODO

<!--URLS-->
[OWNERS]: ./OWNERS
[README]: ./README.md
[playbooks]: ./../docs/playbooks.md
<!--Additional URLS-->
[cluster]: ./cluster/
[prow-k8s-io]: https://prow.k8s.io
[Stackdriver]: https://cloud.google.com/stackdriver/

