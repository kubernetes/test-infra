# Migrating Kubernetes Jobs To The EKS Prow Build Cluster

In an ongoing effort to migrate to community-owned resources, SIG K8S Infra and
SIG Testing are working to complete the migration of jobs from the Google-owned
internal GKE cluster to community-owned clusters.

Most of jobs in the Prow Default Build Cluster should attempt to migrate to
`cluster: eks-prow-build-cluster`. For criteria and requirements, please
pay attention to this document.

## What is EKS Prow Build Cluster?

EKS Prow Build Cluster (`eks-prow-build-cluster`) is an AWS EKS based cluster
owned by community that's used for running ProwJobs. It's supposed to be
similar to our GCP/GKE based clusters, but there are some significant
differences such as:

- Operating system on worker nodes where we run jobs is Ubuntu 20.04 (EKS
  optimized)
- The cluster is smaller in terms of number of worker nodes, but worker nodes
  are larger (16 vCPUs, 128 GB RAM, 300 GB NVMe SSD)
- There's cluster-autoscaler in place to scale up/down cluster on demand
- The cluster is hosted on AWS which means we get to use the credits donation
  that we got from AWS :)

## Criteria and Requirements for Migration

The following jobs can be migrated out of the box:

- Jobs not using any external resources (e.g. cloud accounts)
  - Build, lint, verify, and similar jobs

The following jobs can be migrated but require some actions to be taken:

- Jobs using external non-GCP resources (e.g. DigitalOcean, vSphere...)
  - Community ownership of resources is required for jobs running in the new
    cluster, see the Community Ownership section for more details

The following jobs **MUST NOT** be migrated at this time:

- Jobs using GCP resources (e.g. E2E tests, promotion jobs, etc.)
- Jobs that are running in trusted clusters (e.g. `test-infra-trusted` and
  `k8s-infra-prow-build-trusted`)

Jobs that are already running in community-owned clusters (e.g. 
`k8s-infra-prow-build`) can be migrated as well, but it's not required or
mandated.

## How To Migrate Jobs?

Fork and check out the [kubernetes/test-infra][k-test-infra] repository,
then follow the steps below:

[k-test-infra]: https://github.com/kubernetes/test-infra

- Find a job that you want to migrate
  - You can check out the following ["Prow Results"][prow-results-default] link
    for recent jobs that are running in the `default` cluster
  - For explanation reasons, let's say that you picked up a job called
    `pull-jobset-test-integration-main`
- Edit the file that `pull-jobset-test-integration-main` is defined in. All
  jobs are defined in the [kubernetes/test-infra][k-test-infra] repository. You
  can use GitHub or our [Code Search tool][cs-test-infra] to find the file
  where this job is defined (search by job name).
- Look for a `.spec.cluster` key in the job definition. If there isn't one or
  it's set to `default`, then the job runs in the default cluster. Add (or
  replace) the following `.spec.cluster` key:
  `cluster: eks-prow-build-cluster`.
  - **IMPORTANT: if you see any entries under `label` that says `gce` skip this
    job and get back to it the next time as this is not ready to be moved yet.**
  - **IMPORTANT: if you see that a job uses Boskos, (e.g. there's `BOSKOS_HOST`
    environment variable set), please check with SIG K8s infra if a needed
    Boskos resource is available in the EKS Prow build cluster (see contact
    information at the end of this document).**
  - **IMPORTANT: if you see any entries under `label` or `volumeMounts` that
    might indicate that a job is using some external non-GCP resource (e.g.
    credentials for some cloud platform), you need to check if the Community
    Ownership criteria is satisfied for that resource (see the Community
    Ownership section for more details)**
  - **IMPORTANT: jobs running in community-owned clusters must have resource
    requests and limits specified for cluster-autoscaler to work properly.
    If that's not the case for your job, please see the next section for some
    details about determining correct resource requests and limits.**
  - Here's an [example of a job][example-eks-job] as a reference (pay attention
    to the `.spec.cluster` key)
- Save the file, commit the change, create a branch and file a PR
- Once the PR is merged, follow the guidelines in the Post Migration Period
  section of this document to ensure that the job remains stable

If you have any trouble, please see how you can get in touch with us at the
end of this document.

[prow-results-default]: https://prow.k8s.io/?cluster=default
[cs-test-infra]: https://cs.k8s.io/?q=job-name-here&i=nope&files=config%2F&excludeFiles=&repos=kubernetes/test-infra
[example-eks-job]: https://github.com/kubernetes/test-infra/blob/1d219efcca8a254aaca2c34570db0a56a05f5770/config/jobs/kubernetes/cloud-provider-aws/cloud-provider-aws-config.yaml#L3C1-L31

### Determining Resource Requests and Limits

Jobs running in community-owned clusters must have resource requests and 
limits specified for cluster-autoscaler to work properly. However, determining
this is not always easy and incorrect requests and limits can cause the job
to start flaking or failing.

- If your job has resource requests specified but not limits, set limits
  to same values as requests. That's usually a good starting point, but
  some adjustments might be needed
- Try to determine requests and limits based on what job is doing
  - Simple verification jobs (e.g. `gofmt`, license header checks, etc.)
    generally don't require a lot of resources. In such case, 1-2 vCPUs and
    1-2 GB RAM is usually enough, but that also depends on the size of the
    project
  - Builds and tests takes some more resources. We generally recommend at least
    2-4 vCPUs and 2-4 GB RAM, but that again depends on the project size
  - Lints jobs (e.g. `golangci-lint`) can be very resource intensive depending
    on configuration (e.g. enabled and disabled linters) as well the project
    size. We recommend at least 2-4 vCPUs and 4-8 GB RAM for lint jobs

At this time, we strongly recommend that you match values for requests and
limits to avoid any potential issues.

Once the job migration PR is merged, make sure to follow guidelines stated in
the Post Migration Period section and adjust requests and limits as needed.

## Post Migration Period

The job should be monitored for its stability after migration. We want to make
sure that stability remains same or improved after the migration. There are
several steps that you should take:

- Watch job's duration and success/flakiness rate over several days
  - You can find information about job runs on the
    ["Prow Results"][prow-results] page. You can also use [Testgrid][testgrid]
    if the job has a testgrid dashboard
  - The job duration and success/flakiness rate should be same or similar as in
    the old cluster. If you notice significant bumps, try adjusting resource
    requests and limits. If that doesn't help, reach out to us so that we
    can investigate
  - Note that some jobs are flaky by their nature, i.e. they were flaking in
    the default cluster too. This is not going to be fixed by moving job to
    the new cluster, but we shouldn't see flakiness rate getting worse
- Watch job's actual resource consumption and adjust requests and limits as
  needed
  - We have a [Grafana dashboard][monitoring-eks] that can show you actual
    resource CPU and memory usage for job. If you notice that CPU gets
    throttled too often, try increasing number of allowed CPUs. Similar for
    memory, if you see memory usage too close too limits, try increasing it
    a little bit

[prow-results]: https://prow.k8s.io
[testgrid]: http://testgrid.k8s.io
[monitoring-eks]: https://monitoring-eks.prow.k8s.io/d/53g2x7OZz/jobs?orgId=1&refresh=30s&var-org=kubernetes&var-repo=kubernetes&var-job=All

## Known Issues

- Golang doesn't respect cgroups, i.e. CPU limits are not going to be respected
  by Golang applications
  - This means that a Go application will try to use all available CPUs on
    node, even though it's limited by (Kubernetes) resource limits, e.g. to 2
  - This can cause massive throttling and performance-sensitive tasks and tests
    can be hit by this. Nodes in the new clusters are much larger
    (we had 8 vCPUs in the old cluster per node, while we have 16 vCPUs in the
    new cluster per node), so it can be easier to get affected by this issue
  - In general, number of tests affected by this should be **very low**.
    However, if you think you're affected by this, you can try to mitigate the
    issue by setting `GOMAXPROCS` environment variable for that job to the
    value of `cpu` limit. There are also ways to automatically determine
    `GOMAXPROCS`, such as [`automaxprocs`][automaxprocs]
- Kernel configs are not available inside the job's test pod so kubeadm might
  show a warning about this
  - We're still working on a permanent resolution, but you can take a look at
    the [following GitHub issue][gh-issue-kubeadm] for more details

[automaxprocs]: https://github.com/uber-go/automaxprocs
[gh-issue-kubeadm]: https://github.com/kubernetes/kubeadm/issues/2898

## Community Ownership of Resources

We require that the external resources used in community-clusters satisfy
the minimum Community Ownership criteria before we add relevant secrets and
credentials to our community-clusters. There are two major reasons for that:

- Ensuring safe and secure pipeline. We want to be able to securely integrate
  the given resource and our clusters, e.g. by generating credentials and
  putting them in the cluster
- Continuity. We want to make sure that we don't lose access to the resource
  in case someone steps down from the project or becomes unreachable

The minimum Community Ownership criteria is as follows:

- SIG K8s infra leadership **MUST** have access to the given external resource 
  - This means that the leadership team must be given access and onboarded to
    the resource (e.g. cloud platform) so they can maintain the access and
    generate secrets to be used in the build cluster

The recommend Community Ownership criteria is as follows:

- We recommend going through the [donation process with CNCF][cncf-credits]
  so that we have proper track of resources available to us, and that you also
  get highlighted for your donation and help to the project
  - In case you want to go through this process, please reach out to SIG K8s
    infra, so that we connect you with CNCF and follow you through the process
  - However, we understand that this require additional effort and is not
    always possible, hence there's minimum criteria to ensure we don't
    block on migration

[sig-k8s-infra-leads]: https://github.com/kubernetes/community/tree/master/sig-k8s-infra#leadership
[cncf-credits]: https://www.cncf.io/credits/

## Reporting Issues and Getting in Touch

If you encounter any issue along the way, we recommend leaving a comment
in our [tracking GitHub issue][test-infra-gh-issue]. You can also reach out
to us:

- via our Slack channels: [#sig-k8s-infra][slack-k8s-infra] and
  [#sig-testing][slack-sig-testing]
- via our mailing lists: `kubernetes-sig-k8s-infra@googlegroups.com` and
  `kubernetes-sig-testing@googlegroups.com`

[test-infra-gh-issue]: https://github.com/kubernetes/test-infra/issues/29722
[slack-k8s-infra]: https://kubernetes.slack.com/archives/CCK68P2Q2
[slack-sig-testing]: https://kubernetes.slack.com/archives/C09QZ4DQB
