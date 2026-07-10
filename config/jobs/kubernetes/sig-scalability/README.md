# GCE 5000-node (5k) scale jobs

## The jobs

There are four 5k jobs: a release-blocking 5k and a 5k experimental, each with a
periodic and a presubmit. The experimental jobs run an experiment and may carry
changes we are testing before landing them on the release-blocking 5k job.

- `ci-kubernetes-e2e-gce-scale-performance-5000` (periodic, release-blocking, runs every other day)
- `pull-kubernetes-gce-master-scale-performance-5000` (presubmit)
- `ci-kubernetes-e2e-gce-scale-performance-5000-experimental` (periodic, twice a week)
- `pull-kubernetes-gce-master-scale-performance-5000-experimental` (presubmit)

All four run the same scale test scenario from kubernetes/kops.

The experimental track exists to de-risk a change against a real 5k cluster
before it affects the release-blocking signal. Any large behavior change lands on
the experimental 5k job first and is only graduated to the release-blocking job
once it clears a passing bar.

## The experiment variant

The experimental jobs set an `EXPERIMENT_VARIANT` env var that the kops scenario
script reads to layer one named experiment on top of the standard 5k config, with
the variant shown as a column on the [TestGrid tab](https://testgrid.k8s.io/sig-scalability-gce#pull-kubernetes-gce-master-scale-performance-5000-experimental).

Keeping the experiment behind a variant lets us easily identify which experiment
a run on TestGrid corresponds to.

## Promoting experimental -> release-blocking

1. Check with SIG Scalability on Slack first, to make sure the experiment slot
   isn't already in use by another contributor.
2. Update the experiment behind `EXPERIMENT_VARIANT` on the experimental periodic
   and presubmit, keeping the two in sync.
3. **Graduation bar:** once the experiment has 4 passing runs in a row, it is
   considered stable enough to graduate.
4. Fold the variant's settings into the release-blocking job
   `ci-kubernetes-e2e-gce-scale-performance-5000` and drop the variant.

## Experiment history

A running log of what has been or is being staged through the experimental 5k
track. Add new entries at the top.

| Variant | Date | What it does |
| --- | --- | --- |
| `restart-etcd36` | 2026-07-10 | RangeStream (`+EtcdRangeStream`) with HA control plane restart, on etcd 3.6.12, to gather data on RangeStream against etcd 3.6. Presubmit only. |
| `restart` | 2026-06-29 | HA control plane with restart, for tracking improvements with watch cache initialization. |
| `etcd36` | 2026-06-26 | Testing the 5k job on etcd 3.6.12. |
