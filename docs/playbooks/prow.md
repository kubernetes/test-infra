# Prow Playbook

This is the playbook for Prow. See also [the playbook index][playbooks].

TDLR: Prow is a set of CI services.

The [prow OWNERS][prow OWNERS] are a potential point of contact for more info.

For in depth details about the project see the [prow README][prow README].

## Prow deployment

Prow is composed of a service cluster, and one or more build clusters
- service cluster: runs prow components, responsible for handling GitHub events and scheduling ProwJob CRDs
- build cluster: runs Pods that implement ProwJob CRDs

Each build cluster may have additional components deployed:
- boskos: responsible for managing pools of GCP projects
- greenhouse: implements a remote bazel cache

Each cluster is a GKE cluster, living in its own GCP project, which may live in separate GCP organizations:
- google.com: the Google-owned GCP project
- kubernetes.io: the community-owned GCP project

### kubernetes prow service cluster aka prow.k8s.io

- The kubernetes prow service cluster, exposed as https://prow.k8s.io
- Lives in google.com GCP project k8s-prow
- Infra manually managed
- Kubernetes manifests live in /config/prow/cluster
- Owner access given to Google employees in test-infra-oncall
- Viewer access given to Google employees
- Logs available via google cloud logging

- tide: merges PRs once label/review requirements satisfied, may re-run tests, may merge a batch of PRs
  - what is tide doing right now: https://prow.k8s.io/tide
  - what has tide been doing: https://prow.k8s.io/tide-history
    - e.g. [tide history for kubernetes/kubernetes master](https://prow.k8s.io/tide-history?repo=kubernetes%2Fkubernetes&branch=master)
    - lots of "TRIGGER_BATCH" with no "MERGE_BATCH" may mean tests are failing/flaking

- plank: schedules Pods implementing ProwJob CRDs
  - dashboard: https://monitoring.prow.k8s.io/d/e1778910572e3552a935c2035ce80369/plank-dashboard
    - plots count of ProwJob CRDs in prow service cluster's registry, filtered/group by relevant fields
    - e.g. [all kubernetes/kubernetes ProwJob CRDs over the last 7d](https://monitoring.prow.k8s.io/d/e1778910572e3552a935c2035ce80369/plank-dashboard?orgId=1&from=now-7d&to=now&var-cluster=All&var-org=kubernetes&var-repo=kubernetes&var-state=$__all&var-type=$__all&var-group_by_1=type&var-group_by_2=state&var-group_by_3=cluster)


### default

- The default prow build cluster
- Lives in google.com GCP project k8s-prow-builds
- Infra manually managed
- Kubernetes manifests live in /config/prow/cluster
- Owner access given to Google employees in test-infra-oncall
- Viewer access given to Google employees
- Runs boskos
- Runs greenhouse
- Logs available via google cloud logging

### test-infra-trusted

- The google.com-owned build cluster for trusted jobs that need access to sensitive secrets
- Is the kubernetes prow service cluster, under a different name

### k8s-infra-prow-build

- The community-owned prow build cluster
- Lives in kubernetes.io GCP project k8s-infra-prow-build
- Infra managed via terraform in k8s.io/infra/gcp/clusters/projects/k8s-infra-prow-build/prow-build
- Kubernetes manifests live in k8s.io/infra/gcp/clusters/projects/k8s-infra-prow-build/prow-build/resources
- Owner access given to k8s-infra-prow-oncall@kubernetes.io
- Viewer access given to k8s-infra-prow-viewers@kubernetes.io
- Kubernetes API access restricted to internal networks, must use google cloud shell
- Runs boskos
- Runs greenhouse
- [k8s-infra-prow-build dashboard](https://console.cloud.google.com/monitoring/dashboards/custom/10925237040785467832?project=k8s-infra-prow-build&timeDomain=1d)
- [k8s-infra-prow-build logs](https://console.cloud.google.com/logs/query?project=k8s-infra-prow-build)

### k8s-infra-prow-build-trusted

- The community-owned prow build cluster for trusted jobs that need access to sensitive secrets
- Lives in kubernetes.io GCP project k8s-infra-prow-build-trusted
- Infra managed via terraform in k8s.io/infra/gcp/clusters/projects/k8s-infra-prow-build-trusted/prow-build-trusted
- Kubernetes manifests live in k8s.io/infra/gcp/clusters/projects/k8s-infra-prow-build-trusted/prow-build-trusted/resources
- Owner access given k8s-infra-prow-oncall@kubernetes.io
- Viewer access given to k8s-infra-prow-viewers@kubernetes.io
- Kubernetes API access restricted to internal networks, must use google cloud shell
- [k8s-infra-prow-build-trusted logs](https://console.cloud.google.com/logs/query?project=k8s-infra-prow-build-trusted)

### others

- There are other build clusters not directly related to kubernetes CI, e.g. kubeflow

### Logs

All cluster logs are accessible via google cloud logging. Access to logs requires Viewer access for the cluster's project.

If you are a googler checking prow.k8s.io, you may open `go/prow-debug` in your
browser. If you are not a googler but have access to this prow, you can
open [Stackdriver] logs in the `k8s-prow` GCP projects.

Other prow deployments may have their own logging stack.

### Monitoring

TODO - prow:
- monitoring.prow.k8s.io
- prow.k8s.io
- tide history
- tide status
k8s-infra-prow-build-trusted:
- TODO

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
[prow OWNERS]: /prow/OWNERS
[prow README]: /prow/README.md
[playbooks]: /docs/playbooks/README.md
<!--Additional URLS-->
[cluster]: ./cluster/
[prow-k8s-io]: https://prow.k8s.io
[Stackdriver]: https://cloud.google.com/stackdriver/

[k8s-infra/prowjob-resource-usage]: https://console.cloud.google.com/monitoring/dashboards/custom/10510319052103514664?authuser=1&project=k8s-infra-prow-build&timeDomain=1d
[k8s-infra/prow-build]: https://console.cloud.google.com/monitoring/dashboards/custom/10510319052103514664?authuser=1&project=k8s-infra-prow-build&timeDomain=1d
