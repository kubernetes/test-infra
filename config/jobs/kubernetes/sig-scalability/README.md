# Kubernetes Scalability Jobs

The Kubernetes Project runs 5000 node clusters to test scalability. These jobs run on AWS and GCP using kOps.

## The jobs

| Job Name  | Cloud | Frequency  |  Experimental |  Owners | Node Count | Notes |
|---|---|---|---|---|---|---|
| ci-kubernetes-e2e-gce-scale-performance-5000  |  GCP  | Every other day  |  No |  #sig-scalability | 5000 | 
| ec2-master-scale-performance |  AWS  | Every other day  |  No |  #sig-scalability | 5000 |
| ec2-dra-with-workload-master-scalability-5000 |  AWS  | Every other day |  No |  #sig-node | 5000 |
| gce-master-scale-performance-5000-experimental | GCP | Twice a week | Yes | #sig-scalability | 5000 |
| gce-master-scale-correctness |  GCP  | Every other day  |  No |  #sig-scalability | 2000 |
| ec2-dra-with-workload-master-scalability-500 |  AWS  | Once a day |  No |  #sig-node | 500 |
|gce-dra-extended-resources-with-workload-master-scalability-100 | GCP | Once a day |  No |  #sig-node | 100 |
| ec2-master-small-scale-performance |  AWS  | 4 times a day  |  No |  #sig-scalability | 100 |
| gce-master-scale-performance-100 | GCP | Every 30 minutes | No | #sig-scalability | 100 |
| azure-master-scalability-100 | Azure | Twice a day | No | #sig-cluster-lifecycle (capz) | 100 |
| golang-tip-k8s-master | GCP | Once a day | No | #sig-scalability | 200* | This job uses the legacy kubemark harness and needs to be rebuilt |

In addition to these jobs, we have presubmit equivalents in kubernetes/kubernetes, kubernetes/perf-tests and kubernetes/kops to debug as needed.

## Adding scalability jobs

All jobs that use clusters with more than 50 nodes are considered a scalability job and require approvals from:
- SIG K8s Infra
- SIG Scalability
- The SIG that owns the subproject

There is a mandatory requirement to use the kops harness as it allows us to run the job on GCP, AWS and Azure. The legacy kubeup harness & kubemark are deprecated and we are not accepting it for new jobs.

You'll need to bring a proposal to this group explaining what you are testing and the benefit to the ecosystem.

The new job must be listed in the README.md

## Keepings jobs green and updated

We expect the owners of the jobs to keep their jobs green and fix failing tests quickly.

Jobs will be reviewed regularly and the jobs that are perma-failing will be removed.

## Experiments

The experimental track exists to de-risk a change against a real 5k cluster
before it affects the release-blocking signal. Any large behavior change lands on
the experimental 5k job first and is only graduated to the release-blocking job
once it clears a passing bar. We welcome experiments but you'll need to coordinate with sig-scalability as we are doing one experiment at a time.

### The experiment variant

The experimental jobs set an `EXPERIMENT_VARIANT` env var that the kops scenario
script reads to layer one named experiment on top of the standard 5k config, with
the variant shown as a column on the [TestGrid tab](https://testgrid.k8s.io/sig-scalability-gce#pull-kubernetes-gce-master-scale-performance-5000-experimental).

Keeping the experiment behind a variant lets us easily identify which experiment
a run on TestGrid corresponds to.

### Promoting experimental -> release-blocking

1. Check with SIG Scalability on Slack first, to make sure the experiment slot
   isn't already in use by another contributor.
2. Update the experiment behind `EXPERIMENT_VARIANT` on the experimental periodic
   and presubmit, keeping the two in sync.
3. **Graduation bar:** once the experiment has 4 passing runs in a row, it is
   considered stable enough to graduate.
4. Fold the variant's settings into the release-blocking job
   `ci-kubernetes-e2e-gce-scale-performance-5000` and drop the variant.

### Experiment history

A running log of what has been or is being staged through the experimental 5k
track. Add new entries at the top.

| Variant | Date | What it does | Owner | Status |
| --- | --- | --- | --- | --- |
| `restart-etcd36` | 2026-07-10 | RangeStream (`+EtcdRangeStream`) with HA control plane restart, on etcd 3.6.12, to gather data on RangeStream against etcd 3.6. Presubmit only. | @Jefftree | Completed |
| `restart` | 2026-06-29 | HA control plane with restart, for tracking improvements with watch cache initialization. |  @Jefftree | In Progress |
| `etcd36` | 2026-06-26 | Testing the 5k job on etcd 3.6.12. | @Jefftree | Completed |
