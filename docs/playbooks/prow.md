# Prow Playbook

This is the playbook for Prow. See also [the playbook index][playbooks].

TDLR: Prow is a set of CI services.

The [prow OWNERS][prow OWNERS] are a potential point of contact for more info.

For in depth details about the project see the [prow README][prow README].

## Prow deployment

Prow is composed of a service cluster, and one or more build clusters
- **service cluster**: runs prow components, responsible for handling GitHub
  events and scheduling ProwJob CRDs
- **build cluster**: runs Pods that implement ProwJob CRDs

Each build cluster may have additional components deployed:
- **boskos**: responsible for managing pools of GCP projects
- **greenhouse**: implements a remote bazel cache
- **ghproxy**: reverse proxy HTTP cache optimized for use with the GitHub API
- **kubernetes-external-secrets**: updates Kubernetes Secrets with values from 
  external secret stores such as Google Secret Manager

Each cluster is a GKE cluster, living in its own GCP project, which may live
in separate GCP organizations:
- **google.com**: the Google-owned GCP project
- **kubernetes.io**: the community-owned GCP project

### kubernetes prow service cluster aka prow.k8s.io

- The kubernetes prow service cluster, exposed as https://prow.k8s.io
- Lives in google.com GCP project k8s-prow
- Infra manually managed
- Kubernetes manifests live in /config/prow/cluster
- Owner access given to Google employees in test-infra-oncall
- Viewer access given to Google employees
- Logs available via Google Cloud Logging

### default

- The default prow build cluster for prow.k8s.io
- Lives in google.com GCP project k8s-prow-builds
- Infra manually managed
- Kubernetes manifests live in /config/prow/cluster
- Owner access given to Google employees in test-infra-oncall
- Viewer access given to Google employees
- Additional components: boskos, greenhouse
- Logs available via Google Cloud Logging

### test-infra-trusted

- The google.com-owned build cluster for trusted jobs that need access to sensitive secrets
- Is the kubernetes prow service cluster, under a different name

### k8s-infra-prow-build

- The community-owned prow build cluster
- Lives in kubernetes.io GCP project k8s-infra-prow-build
- Infra managed via terraform in k8s.io/infra/gcp/terraform/k8s-infra-prow-build/prow-build
- Kubernetes manifests live in k8s.io/infra/gcp/terraform/k8s-infra-prow-build/prow-build/resources
- Owner access given to k8s-infra-prow-oncall@kubernetes.io
- Viewer access given to k8s-infra-prow-viewers@kubernetes.io
- Kubernetes API access restricted to internal networks, must use google cloud shell
- Additional components: boskos, greenhouse, kubernetes-external-secrets
- [k8s-infra-prow-build dashboard](https://console.cloud.google.com/monitoring/dashboards/custom/10925237040785467832?project=k8s-infra-prow-build&timeDomain=1d)
- [k8s-infra-prow-build logs](https://console.cloud.google.com/logs/query?project=k8s-infra-prow-build)

### k8s-infra-prow-build-trusted

- The community-owned prow build cluster for trusted jobs that need access to sensitive secrets
- Lives in kubernetes.io GCP project k8s-infra-prow-build-trusted
- Infra managed via terraform in k8s.io/infra/gcp/terraform/k8s-infra-prow-build-trusted/prow-build-trusted
- Kubernetes manifests live in k8s.io/infra/gcp/terraform/k8s-infra-prow-build-trusted/prow-build-trusted/resources
- Owner access given k8s-infra-prow-oncall@kubernetes.io
- Viewer access given to k8s-infra-prow-viewers@kubernetes.io
- Kubernetes API access restricted to internal networks, must use google cloud shell
- [k8s-infra-prow-build-trusted logs](https://console.cloud.google.com/logs/query?project=k8s-infra-prow-build-trusted)

### others

- There are other prow build clusters that prow.k8s.io currently schedules to
  that are not directly related to kubernetes CI or community-owned,
  e.g. scalability

## Logs

All cluster logs are accessible via google cloud logging. Access to logs
requires Viewer access for the cluster's project.

If you are a googler checking prow.k8s.io, you may open `go/prow-debug` in your
browser. If you are not a googler but have access to this prow, you can
open [Stackdriver] logs in the `k8s-prow` GCP projects.

Other prow deployments may have their own logging stack.

## Monitoring

### Tide dashboards

Tide merges PRs once label/review requirements satisfied, may re-run tests,
may merge a batch of PRs

- What is tide doing right now: https://prow.k8s.io/tide
- What has tide been doing: https://prow.k8s.io/tide-history
  - e.g. [tide history for kubernetes/kubernetes master](https://prow.k8s.io/tide-history?repo=kubernetes%2Fkubernetes&branch=master)
  - lots of "TRIGGER_BATCH" with no "MERGE_BATCH" may mean tests are failing/flaking

### ProwJob dashboards

ProwJobs are CRDs, updated based on the status of whatever is responsible for
implementing them (usually Kubernetes Pods scheduled to a prow build cluster).
Due to the volume of traffic prow.k8s.io handles, and in order to keep things
responsive, ProwJob CRDs are only retained for 48h. This affects the amount of
history available on the following dashboards

- How many ProwJob CRDs exist right now: https://monitoring.prow.k8s.io/d/e1778910572e3552a935c2035ce80369/plank-dashboard
  - e.g. [all kubernetes/kubernetes ProwJob CRDs over the last 7d](https://monitoring.prow.k8s.io/d/e1778910572e3552a935c2035ce80369/plank-dashboard?orgId=1&from=now-7d&to=now&var-cluster=All&var-org=kubernetes&var-repo=kubernetes&var-state=$__all&var-type=$__all&var-group_by_1=type&var-group_by_2=state&var-group_by_3=cluster)
  - plots count of ProwJob CRDs in prow service cluster's registry, filtered/group by relevant fields

- What ProwJobs are scheduled right now: https://prow.k8s.io/
  - e.g. [all prowjobs running as pods in k8s-infra-prow-build](https://prow.k8s.io/?cluster=k8s-infra-prow-build)
  - e.g. [all kubernetes/kubernetes presubmits](https://prow.k8s.io/?repo=kubernetes%2Fkubernetes&type=presubmit)

### Build cluster dashboards

Access to these dashboards requires Viewer access for the cluster's project.
This is available to members of k8s-infra-prow-oncall@kubernetes.io and
k8s-infra-prow-viewers@kubernetes.io

- What resources is k8s-infra-prow-build using right now: https://console.cloud.google.com/monitoring/dashboards/builder/10925237040785467832?project=k8s-infra-prow-build&timeDomain=6h
- What resources are used by jobs on k8s-infra-prow-build: https://console.cloud.google.com/monitoring/dashboards/builder/10510319052103514664?project=k8s-infra-prow-build&timeDomain=1h

## Options

The following well-known options are available for dealing with prow
service issues.

### Rolling Back

For prow.k8s.io you can simply use `experiment/revert-bump.sh` to roll back
to the last checked in deployment version.

If prow is at least somewhat healthy, filing and merging PR from this will 
result in the rolled back version being deployed.

If not, you may need to manually run `make -C config/prow deploy-all`.


## Known Issues


<!--URLS-->
[prow OWNERS]: /prow/OWNERS
[prow README]: /prow/README.md
[playbooks]: /docs/playbooks/README.md
<!--Additional URLS-->
[cluster]: /config/cluster
[prow-k8s-io]: https://prow.k8s.io
[Stackdriver]: https://cloud.google.com/stackdriver/

[k8s-infra/prowjob-resource-usage]: https://console.cloud.google.com/monitoring/dashboards/custom/10510319052103514664?authuser=1&project=k8s-infra-prow-build&timeDomain=1d
[k8s-infra/prow-build]: https://console.cloud.google.com/monitoring/dashboards/custom/10510319052103514664?authuser=1&project=k8s-infra-prow-build&timeDomain=1d
