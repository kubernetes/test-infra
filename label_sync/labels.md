# Kubernetes GitHub Labels

## Table of Contents

- [Intro](#intro)
  - [Why these labels?](#why-these-labels)
  - [How do I add a new label?](#how-do-i-add-a-new-label)
- [Labels that apply to all repos, for both issues and PRs](#labels-that-apply-to-all-repos-for-both-issues-and-prs)
- [Labels that apply to all repos, only for issues](#labels-that-apply-to-all-repos-only-for-issues)
- [Labels that apply to all repos, only for PRs](#labels-that-apply-to-all-repos-only-for-prs)
- [Labels that apply to kubernetes-sigs/cluster-api, for both issues and PRs](#labels-that-apply-to-kubernetes-sigscluster-api-for-both-issues-and-prs)
- [Labels that apply to kubernetes-sigs/cluster-api, only for PRs](#labels-that-apply-to-kubernetes-sigscluster-api-only-for-prs)
- [Labels that apply to kubernetes-sigs/cluster-api-operator, for both issues and PRs](#labels-that-apply-to-kubernetes-sigscluster-api-operator-for-both-issues-and-prs)
- [Labels that apply to kubernetes-sigs/cluster-api-provider-aws, for both issues and PRs](#labels-that-apply-to-kubernetes-sigscluster-api-provider-aws-for-both-issues-and-prs)
- [Labels that apply to kubernetes-sigs/cluster-api-provider-aws, only for issues](#labels-that-apply-to-kubernetes-sigscluster-api-provider-aws-only-for-issues)
- [Labels that apply to kubernetes-sigs/cluster-api-provider-azure, for both issues and PRs](#labels-that-apply-to-kubernetes-sigscluster-api-provider-azure-for-both-issues-and-prs)
- [Labels that apply to kubernetes-sigs/cluster-api-provider-gcp, for both issues and PRs](#labels-that-apply-to-kubernetes-sigscluster-api-provider-gcp-for-both-issues-and-prs)
- [Labels that apply to kubernetes-sigs/cluster-api-provider-vsphere, for both issues and PRs](#labels-that-apply-to-kubernetes-sigscluster-api-provider-vsphere-for-both-issues-and-prs)
- [Labels that apply to kubernetes-sigs/contributor-tweets, for both issues and PRs](#labels-that-apply-to-kubernetes-sigscontributor-tweets-for-both-issues-and-prs)
- [Labels that apply to kubernetes-sigs/controller-runtime, for both issues and PRs](#labels-that-apply-to-kubernetes-sigscontroller-runtime-for-both-issues-and-prs)
- [Labels that apply to kubernetes-sigs/gateway-api, only for issues](#labels-that-apply-to-kubernetes-sigsgateway-api-only-for-issues)
- [Labels that apply to kubernetes-sigs/gateway-api, only for PRs](#labels-that-apply-to-kubernetes-sigsgateway-api-only-for-prs)
- [Labels that apply to kubernetes-sigs/kind, for both issues and PRs](#labels-that-apply-to-kubernetes-sigskind-for-both-issues-and-prs)
- [Labels that apply to kubernetes-sigs/krew, for both issues and PRs](#labels-that-apply-to-kubernetes-sigskrew-for-both-issues-and-prs)
- [Labels that apply to kubernetes-sigs/kubespray, only for PRs](#labels-that-apply-to-kubernetes-sigskubespray-only-for-prs)
- [Labels that apply to kubernetes-sigs/promo-tools, for both issues and PRs](#labels-that-apply-to-kubernetes-sigspromo-tools-for-both-issues-and-prs)
- [Labels that apply to kubernetes/community, for both issues and PRs](#labels-that-apply-to-kubernetescommunity-for-both-issues-and-prs)
- [Labels that apply to kubernetes/dns, only for issues](#labels-that-apply-to-kubernetesdns-only-for-issues)
- [Labels that apply to kubernetes/enhancements, for both issues and PRs](#labels-that-apply-to-kubernetesenhancements-for-both-issues-and-prs)
- [Labels that apply to kubernetes/enhancements, only for issues](#labels-that-apply-to-kubernetesenhancements-only-for-issues)
- [Labels that apply to kubernetes/k8s.io, for both issues and PRs](#labels-that-apply-to-kubernetesk8s.io-for-both-issues-and-prs)
- [Labels that apply to kubernetes/k8s.io, only for PRs](#labels-that-apply-to-kubernetesk8s.io-only-for-prs)
- [Labels that apply to kubernetes/kubeadm, for both issues and PRs](#labels-that-apply-to-kuberneteskubeadm-for-both-issues-and-prs)
- [Labels that apply to kubernetes/kubernetes, for both issues and PRs](#labels-that-apply-to-kuberneteskubernetes-for-both-issues-and-prs)
- [Labels that apply to kubernetes/kubernetes, only for issues](#labels-that-apply-to-kuberneteskubernetes-only-for-issues)
- [Labels that apply to kubernetes/kubernetes, only for PRs](#labels-that-apply-to-kuberneteskubernetes-only-for-prs)
- [Labels that apply to kubernetes/org, for both issues and PRs](#labels-that-apply-to-kubernetesorg-for-both-issues-and-prs)
- [Labels that apply to kubernetes/org, only for issues](#labels-that-apply-to-kubernetesorg-only-for-issues)
- [Labels that apply to kubernetes/release, for both issues and PRs](#labels-that-apply-to-kubernetesrelease-for-both-issues-and-prs)
- [Labels that apply to kubernetes/sig-release, for both issues and PRs](#labels-that-apply-to-kubernetessig-release-for-both-issues-and-prs)
- [Labels that apply to kubernetes/sig-security, for both issues and PRs](#labels-that-apply-to-kubernetessig-security-for-both-issues-and-prs)
- [Labels that apply to kubernetes/test-infra, for both issues and PRs](#labels-that-apply-to-kubernetestest-infra-for-both-issues-and-prs)
- [Labels that apply to kubernetes/test-infra, only for PRs](#labels-that-apply-to-kubernetestest-infra-only-for-prs)
- [Labels that apply to kubernetes/website, for both issues and PRs](#labels-that-apply-to-kuberneteswebsite-for-both-issues-and-prs)
- [Labels that apply to kubernetes/website, only for issues](#labels-that-apply-to-kuberneteswebsite-only-for-issues)
- [Labels that apply to kubernetes/website, only for PRs](#labels-that-apply-to-kuberneteswebsite-only-for-prs)


## Intro

This file was auto generated by the [label_sync](https://git.k8s.io/test-infra/label_sync/) tool,
based on the [labels.yaml](https://git.k8s.io/test-infra/label_sync/labels.yaml) that it uses to
sync github labels across repos in the [kubernetes github org](https://github.com/kubernetes)

### Why these labels?

The rule of thumb is that labels are here because they are intended to be produced or consumed by
our automation (primarily prow) across all repos. There are some labels that can only be manually
applied/removed, and where possible we would rather remove them or add automation to allow a
larger set of contributors to apply/remove them.

### How do I add a new label?

- Add automation that consumes/produces the label
- Open a PR, _with a single commit_, that:
  - updates [labels.yaml](https://git.k8s.io/test-infra/label_sync/labels.yaml) with the new label(s)
  - runs `make update-labels` from the repo root (to regenerate the label descriptions and associated CSS)
- Involve [sig-contributor-experience](https://git.k8s.io/community/sig-contributor-experience) in the change, eg: chat about it in slack, mention @kubernetes/sig-contributor-experience-pr-reviews in the PR, etc.
- After the PR is merged, a kubernetes CronJob is responsible for syncing labels daily


## Labels that apply to all repos, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="api-review" href="#api-review">`api-review`</a> | Categorizes an issue or PR as actively needing an API review.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/community-meeting" href="#area/community-meeting">`area/community-meeting`</a> | Issues or PRs that should potentially be discussed in a Kubernetes community meeting.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/dependency" href="#area/dependency">`area/dependency`</a> | Issues or PRs related to dependency changes| label | |
| <a id="area/provider/aws" href="#area/provider/aws">`area/provider/aws`</a> | Issues or PRs related to aws provider <br><br> This was previously `area/platform/aws`, `area/platform/eks`, `sig/aws`, `aws`, | label | |
| <a id="area/provider/azure" href="#area/provider/azure">`area/provider/azure`</a> | Issues or PRs related to azure provider <br><br> This was previously `area/platform/aks`, `area/platform/azure`, `sig/azure`, `azure`, | label | |
| <a id="area/provider/digitalocean" href="#area/provider/digitalocean">`area/provider/digitalocean`</a> | Issues or PRs related to digitalocean provider| label | |
| <a id="area/provider/gcp" href="#area/provider/gcp">`area/provider/gcp`</a> | Issues or PRs related to gcp provider <br><br> This was previously `area/platform/gcp`, `area/platform/gke`, `sig/gcp`, `gcp`, | label | |
| <a id="area/provider/ibmcloud" href="#area/provider/ibmcloud">`area/provider/ibmcloud`</a> | Issues or PRs related to ibmcloud provider <br><br> This was previously `sig/ibmcloud`, `ibmcloud`, | label | |
| <a id="area/provider/openstack" href="#area/provider/openstack">`area/provider/openstack`</a> | Issues or PRs related to openstack provider <br><br> This was previously `sig/openstack`, `openstack`, | label | |
| <a id="area/provider/vmware" href="#area/provider/vmware">`area/provider/vmware`</a> | Issues or PRs related to vmware provider <br><br> This was previously `area/platform/vsphere`, `sig/vmware`, `vmware`, | label | |
| <a id="committee/code-of-conduct" href="#committee/code-of-conduct">`committee/code-of-conduct`</a> | Denotes an issue or PR intended to be handled by the code of conduct committee. <br><br> This was previously `committee/conduct`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="committee/security-response" href="#committee/security-response">`committee/security-response`</a> | Denotes an issue or PR intended to be handled by the product security committee. <br><br> This was previously `committee/product-security`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="committee/steering" href="#committee/steering">`committee/steering`</a> | Denotes an issue or PR intended to be handled by the steering committee.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/api-change" href="#kind/api-change">`kind/api-change`</a> | Categorizes issue or PR as related to adding, removing, or otherwise changing an API <br><br> This was previously `kind/new-api`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/bug" href="#kind/bug">`kind/bug`</a> | Categorizes issue or PR as related to a bug. <br><br> This was previously `bug`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/cleanup" href="#kind/cleanup">`kind/cleanup`</a> | Categorizes issue or PR as related to cleaning up code, process, or technical debt. <br><br> This was previously `kind/friction`, `kind/technical-debt`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/deprecation" href="#kind/deprecation">`kind/deprecation`</a> | Categorizes issue or PR as related to a feature/enhancement marked for deprecation.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/documentation" href="#kind/documentation">`kind/documentation`</a> | Categorizes issue or PR as related to documentation. <br><br> This was previously `kind/old-docs`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/failing-test" href="#kind/failing-test">`kind/failing-test`</a> | Categorizes issue or PR as related to a consistently or frequently failing test. <br><br> This was previously `priority/failing-test`, `kind/e2e-test-failure`, `kind/upgrade-test-failure`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/feature" href="#kind/feature">`kind/feature`</a> | Categorizes issue or PR as related to a new feature. <br><br> This was previously `enhancement`, `kind/enhancement`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/flake" href="#kind/flake">`kind/flake`</a> | Categorizes issue or PR as related to a flaky test.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/regression" href="#kind/regression">`kind/regression`</a> | Categorizes issue or PR as related to a regression from a prior release.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/support" href="#kind/support">`kind/support`</a> | Categorizes issue or PR as a support question. <br><br> This was previously `close/support`, `question`, `triage/support`, | humans | |
| <a id="lifecycle/active" href="#lifecycle/active">`lifecycle/active`</a> | Indicates that an issue or PR is actively being worked on by a contributor. <br><br> This was previously `active`, | anyone |  [lifecycle](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/lifecycle) |
| <a id="lifecycle/frozen" href="#lifecycle/frozen">`lifecycle/frozen`</a> | Indicates that an issue or PR should not be auto-closed due to staleness. <br><br> This was previously `keep-open`, | anyone |  [lifecycle](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/lifecycle) |
| <a id="lifecycle/rotten" href="#lifecycle/rotten">`lifecycle/rotten`</a> | Denotes an issue or PR that has aged beyond stale and will be auto-closed.| anyone or [@fejta-bot](https://github.com/fejta-bot) via [periodic-test-infra-rotten prowjob](https://prow.k8s.io/?job=periodic-test-infra-rotten) |  [lifecycle](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/lifecycle) |
| <a id="lifecycle/stale" href="#lifecycle/stale">`lifecycle/stale`</a> | Denotes an issue or PR has remained open with no activity and has become stale. <br><br> This was previously `stale`, | anyone or [@fejta-bot](https://github.com/fejta-bot) via [periodic-test-infra-stale prowjob](https://prow.k8s.io/?job=periodic-test-infra-stale) |  [lifecycle](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/lifecycle) |
| <a id="needs-sig" href="#needs-sig">`needs-sig`</a> | Indicates an issue or PR lacks a `sig/foo` label and requires one.| prow |  [require-matching-label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/require-matching-label) |
| <a id="needs-triage" href="#needs-triage">`needs-triage`</a> | Indicates an issue or PR lacks a `triage/foo` label and requires one.| prow |  [require-matching-label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/require-matching-label) |
| <a id="priority/awaiting-more-evidence" href="#priority/awaiting-more-evidence">`priority/awaiting-more-evidence`</a> | Lowest priority. Possibly useful, but not yet enough support to actually get it done.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="priority/backlog" href="#priority/backlog">`priority/backlog`</a> | Higher priority than priority/awaiting-more-evidence.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="priority/critical-urgent" href="#priority/critical-urgent">`priority/critical-urgent`</a> | Highest priority. Must be actively worked on as someone's top priority right now.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="priority/important-longterm" href="#priority/important-longterm">`priority/important-longterm`</a> | Important over the long term, but may not be staffed and/or may need multiple releases to complete.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="priority/important-soon" href="#priority/important-soon">`priority/important-soon`</a> | Must be staffed and worked on either currently, or very soon, ideally in time for the next release.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/api-machinery" href="#sig/api-machinery">`sig/api-machinery`</a> | Categorizes an issue or PR as relevant to SIG API Machinery.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/apps" href="#sig/apps">`sig/apps`</a> | Categorizes an issue or PR as relevant to SIG Apps.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/architecture" href="#sig/architecture">`sig/architecture`</a> | Categorizes an issue or PR as relevant to SIG Architecture.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/auth" href="#sig/auth">`sig/auth`</a> | Categorizes an issue or PR as relevant to SIG Auth.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/autoscaling" href="#sig/autoscaling">`sig/autoscaling`</a> | Categorizes an issue or PR as relevant to SIG Autoscaling.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/cli" href="#sig/cli">`sig/cli`</a> | Categorizes an issue or PR as relevant to SIG CLI.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/cloud-provider" href="#sig/cloud-provider">`sig/cloud-provider`</a> | Categorizes an issue or PR as relevant to SIG Cloud Provider.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/cluster-lifecycle" href="#sig/cluster-lifecycle">`sig/cluster-lifecycle`</a> | Categorizes an issue or PR as relevant to SIG Cluster Lifecycle.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/contributor-experience" href="#sig/contributor-experience">`sig/contributor-experience`</a> | Categorizes an issue or PR as relevant to SIG Contributor Experience.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/docs" href="#sig/docs">`sig/docs`</a> | Categorizes an issue or PR as relevant to SIG Docs.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/etcd" href="#sig/etcd">`sig/etcd`</a> | Categorizes an issue or PR as relevant to SIG Etcd.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/instrumentation" href="#sig/instrumentation">`sig/instrumentation`</a> | Categorizes an issue or PR as relevant to SIG Instrumentation.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/k8s-infra" href="#sig/k8s-infra">`sig/k8s-infra`</a> | Categorizes an issue or PR as relevant to SIG K8s Infra. <br><br> This was previously `wg/k8s-infra`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/multicluster" href="#sig/multicluster">`sig/multicluster`</a> | Categorizes an issue or PR as relevant to SIG Multicluster. <br><br> This was previously `sig/federation`, `sig/federation (deprecated - do not use)`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/network" href="#sig/network">`sig/network`</a> | Categorizes an issue or PR as relevant to SIG Network.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/node" href="#sig/node">`sig/node`</a> | Categorizes an issue or PR as relevant to SIG Node.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/release" href="#sig/release">`sig/release`</a> | Categorizes an issue or PR as relevant to SIG Release.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/scalability" href="#sig/scalability">`sig/scalability`</a> | Categorizes an issue or PR as relevant to SIG Scalability.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/scheduling" href="#sig/scheduling">`sig/scheduling`</a> | Categorizes an issue or PR as relevant to SIG Scheduling.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/security" href="#sig/security">`sig/security`</a> | Categorizes an issue or PR as relevant to SIG Security.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/storage" href="#sig/storage">`sig/storage`</a> | Categorizes an issue or PR as relevant to SIG Storage.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/testing" href="#sig/testing">`sig/testing`</a> | Categorizes an issue or PR as relevant to SIG Testing.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/ui" href="#sig/ui">`sig/ui`</a> | Categorizes an issue or PR as relevant to SIG UI.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="sig/windows" href="#sig/windows">`sig/windows`</a> | Categorizes an issue or PR as relevant to SIG Windows.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="triage/accepted" href="#triage/accepted">`triage/accepted`</a> | Indicates an issue or PR is ready to be actively worked on.| org members |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="triage/duplicate" href="#triage/duplicate">`triage/duplicate`</a> | Indicates an issue is a duplicate of other open issue. <br><br> This was previously `close/duplicate`, `duplicate`, | humans | |
| <a id="triage/needs-information" href="#triage/needs-information">`triage/needs-information`</a> | Indicates an issue needs more information in order to work on it. <br><br> This was previously `close/needs-information`, | humans | |
| <a id="triage/not-reproducible" href="#triage/not-reproducible">`triage/not-reproducible`</a> | Indicates an issue can not be reproduced as described. <br><br> This was previously `close/not-reproducible`, | humans | |
| <a id="triage/unresolved" href="#triage/unresolved">`triage/unresolved`</a> | Indicates an issue that can not or will not be resolved. <br><br> This was previously `close/unresolved`, `invalid`, `wontfix`, | humans | |
| <a id="ug/big-data" href="#ug/big-data">`ug/big-data`</a> | Categorizes an issue or PR as relevant to ug-big-data. <br><br> This was previously `sig/big-data`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="ug/vmware" href="#ug/vmware">`ug/vmware`</a> | Categorizes an issue or PR as relevant to ug-vmware.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="wg/api-expression" href="#wg/api-expression">`wg/api-expression`</a> | Categorizes an issue or PR as relevant to WG API Expression. <br><br> This was previously `wg/apply`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="wg/batch" href="#wg/batch">`wg/batch`</a> | Categorizes an issue or PR as relevant to WG Batch.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="wg/data-protection" href="#wg/data-protection">`wg/data-protection`</a> | Categorizes an issue or PR as relevant to WG Data Protection.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="wg/device-management" href="#wg/device-management">`wg/device-management`</a> | Categorizes an issue or PR as relevant to WG Device Management.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="wg/iot-edge" href="#wg/iot-edge">`wg/iot-edge`</a> | Categorizes an issue or PR as relevant to WG IOT Edge.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="wg/multitenancy" href="#wg/multitenancy">`wg/multitenancy`</a> | Categorizes an issue or PR as relevant to WG Multitenancy.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="wg/naming" href="#wg/naming">`wg/naming`</a> | Categorizes an issue or PR as relevant to WG Naming.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="wg/policy" href="#wg/policy">`wg/policy`</a> | Categorizes an issue or PR as relevant to WG Policy.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="wg/reliability" href="#wg/reliability">`wg/reliability`</a> | Categorizes an issue or PR as relevant to WG Reliability| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="wg/serving" href="#wg/serving">`wg/serving`</a> | Categorizes an issue or PR as relevant to WG Serving.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="wg/structured-logging" href="#wg/structured-logging">`wg/structured-logging`</a> | Categorizes an issue or PR as relevant to WG Structured Logging.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="¯\_(ツ)_/¯" href="#¯\_(ツ)_/¯">`¯\_(ツ)_/¯`</a> | ¯\\\_(ツ)_/¯| humans |  [shrug](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/shrug) |

## Labels that apply to all repos, only for issues

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="good first issue" href="#good first issue">`good first issue`</a> | Denotes an issue ready for a new contributor, according to the "help wanted" guidelines. <br><br> This was previously `for-new-contributors`, | anyone |  [help](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/help) |
| <a id="help wanted" href="#help wanted">`help wanted`</a> | Denotes an issue that needs help from a contributor. Must meet "help wanted" guidelines. <br><br> This was previously `help-wanted`, | anyone |  [help](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/help) |
| <a id="tide/merge-blocker" href="#tide/merge-blocker">`tide/merge-blocker`</a> | Denotes an issue that blocks the tide merge queue for a branch while it is open. <br><br> This was previously `merge-blocker`, | humans | |

## Labels that apply to all repos, only for PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="approved" href="#approved">`approved`</a> | Indicates a PR has been approved by an approver from all required OWNERS files.| approvers |  [approve](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/approve) |
| <a id="cherry-pick-approved" href="#cherry-pick-approved">`cherry-pick-approved`</a> | Indicates a cherry-pick PR into a release branch has been approved by the release branch manager. <br><br> This was previously `cherrypick-approved`, | humans | |
| <a id="cncf-cla  no" href="#cncf-cla  no">`cncf-cla: no`</a> | Indicates the PR's author has not signed the CNCF CLA.| prow |  [cla](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/cla) |
| <a id="cncf-cla  yes" href="#cncf-cla  yes">`cncf-cla: yes`</a> | Indicates the PR's author has signed the CNCF CLA.| prow |  [cla](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/cla) |
| <a id="do-not-merge" href="#do-not-merge">`do-not-merge`</a> | DEPRECATED. Indicates that a PR should not merge. Label can only be manually applied/removed.| humans | |
| <a id="do-not-merge/blocked-paths" href="#do-not-merge/blocked-paths">`do-not-merge/blocked-paths`</a> | Indicates that a PR should not merge because it touches files in blocked paths.| prow |  [blockade](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/blockade) |
| <a id="do-not-merge/cherry-pick-not-approved" href="#do-not-merge/cherry-pick-not-approved">`do-not-merge/cherry-pick-not-approved`</a> | Indicates that a PR is not yet approved to merge into a release branch.| prow |  [cherrypickunapproved](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/cherrypickunapproved) |
| <a id="do-not-merge/hold" href="#do-not-merge/hold">`do-not-merge/hold`</a> | Indicates that a PR should not merge because someone has issued a /hold command.| anyone |  [hold](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/hold) |
| <a id="do-not-merge/invalid-commit-message" href="#do-not-merge/invalid-commit-message">`do-not-merge/invalid-commit-message`</a> | Indicates that a PR should not merge because it has an invalid commit message.| prow |  [invalidcommitmsg](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/invalidcommitmsg) |
| <a id="do-not-merge/invalid-owners-file" href="#do-not-merge/invalid-owners-file">`do-not-merge/invalid-owners-file`</a> | Indicates that a PR should not merge because it has an invalid OWNERS file in it.| prow |  [verify-owners](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/verify-owners) |
| <a id="do-not-merge/release-note-label-needed" href="#do-not-merge/release-note-label-needed">`do-not-merge/release-note-label-needed`</a> | Indicates that a PR should not merge because it's missing one of the release note labels. <br><br> This was previously `release-note-label-needed`, | prow |  [releasenote](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/releasenote) |
| <a id="do-not-merge/work-in-progress" href="#do-not-merge/work-in-progress">`do-not-merge/work-in-progress`</a> | Indicates that a PR should not merge because it is a work in progress.| prow |  [wip](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/wip) |
| <a id="lgtm" href="#lgtm">`lgtm`</a> | "Looks good to me", indicates that a PR is ready to be merged.| reviewers or members |  [lgtm](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/lgtm) |
| <a id="needs-kind" href="#needs-kind">`needs-kind`</a> | Indicates a PR lacks a `kind/foo` label and requires one.| prow |  [require-matching-label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/require-matching-label) |
| <a id="needs-ok-to-test" href="#needs-ok-to-test">`needs-ok-to-test`</a> | Indicates a PR that requires an org member to verify it is safe to test.| prow |  [trigger](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/trigger) |
| <a id="needs-rebase" href="#needs-rebase">`needs-rebase`</a> | Indicates a PR cannot be merged because it has merge conflicts with HEAD.| prow |  [needs-rebase](https://github.com/kubernetes-sigs/prow/tree/main/cmd/external-plugins/needs-rebase) |
| <a id="ok-to-test" href="#ok-to-test">`ok-to-test`</a> | Indicates a non-member PR verified by an org member that is safe to test.| prow |  [trigger](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/trigger) |
| <a id="release-note" href="#release-note">`release-note`</a> | Denotes a PR that will be considered when it comes time to generate release notes.| prow |  [releasenote](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/releasenote) |
| <a id="release-note-action-required" href="#release-note-action-required">`release-note-action-required`</a> | Denotes a PR that introduces potentially breaking changes that require user action.| prow |  [releasenote](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/releasenote) |
| <a id="release-note-none" href="#release-note-none">`release-note-none`</a> | Denotes a PR that doesn't merit a release note.| prow or member or author |  [releasenote](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/releasenote) |
| <a id="size/L" href="#size/L">`size/L`</a> | Denotes a PR that changes 100-499 lines, ignoring generated files.| prow |  [size](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/size) |
| <a id="size/M" href="#size/M">`size/M`</a> | Denotes a PR that changes 30-99 lines, ignoring generated files.| prow |  [size](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/size) |
| <a id="size/S" href="#size/S">`size/S`</a> | Denotes a PR that changes 10-29 lines, ignoring generated files.| prow |  [size](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/size) |
| <a id="size/XL" href="#size/XL">`size/XL`</a> | Denotes a PR that changes 500-999 lines, ignoring generated files.| prow |  [size](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/size) |
| <a id="size/XS" href="#size/XS">`size/XS`</a> | Denotes a PR that changes 0-9 lines, ignoring generated files.| prow |  [size](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/size) |
| <a id="size/XXL" href="#size/XXL">`size/XXL`</a> | Denotes a PR that changes 1000+ lines, ignoring generated files.| prow |  [size](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/size) |
| <a id="tide/merge-method-merge" href="#tide/merge-method-merge">`tide/merge-method-merge`</a> | Denotes a PR that should use a standard merge by tide when it merges.| humans | |
| <a id="tide/merge-method-rebase" href="#tide/merge-method-rebase">`tide/merge-method-rebase`</a> | Denotes a PR that should be rebased by tide when it merges.| humans | |
| <a id="tide/merge-method-squash" href="#tide/merge-method-squash">`tide/merge-method-squash`</a> | Denotes a PR that should be squashed by tide when it merges. <br><br> This was previously `tide/squash`, | humans | |

## Labels that apply to kubernetes-sigs/cluster-api, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/api" href="#area/api">`area/api`</a> | Issues or PRs related to the APIs| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/bootstrap" href="#area/bootstrap">`area/bootstrap`</a> | Issues or PRs related to bootstrap providers| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/ci" href="#area/ci">`area/ci`</a> | Issues or PRs related to ci| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/clustercachetracker" href="#area/clustercachetracker">`area/clustercachetracker`</a> | Issues or PRs related to the clustercachetracker| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/clusterclass" href="#area/clusterclass">`area/clusterclass`</a> | Issues or PRs related to clusterclass| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/clusterctl" href="#area/clusterctl">`area/clusterctl`</a> | Issues or PRs related to clusterctl| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/clusterresourceset" href="#area/clusterresourceset">`area/clusterresourceset`</a> | Issues or PRs related to clusterresourcesets| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/control-plane" href="#area/control-plane">`area/control-plane`</a> | Issues or PRs related to control-plane lifecycle management| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/devtools" href="#area/devtools">`area/devtools`</a> | Issues or PRs related to devtools| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/documentation" href="#area/documentation">`area/documentation`</a> | Issues or PRs related to documentation| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/e2e-testing" href="#area/e2e-testing">`area/e2e-testing`</a> | Issues or PRs related to e2e testing| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/ipam" href="#area/ipam">`area/ipam`</a> | Issues or PRs related to ipam| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/logging" href="#area/logging">`area/logging`</a> | Issues or PRs related to logging| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/machine" href="#area/machine">`area/machine`</a> | Issues or PRs related to machine lifecycle management| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/machinedeployment" href="#area/machinedeployment">`area/machinedeployment`</a> | Issues or PRs related to machinedeployments| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/machinehealthcheck" href="#area/machinehealthcheck">`area/machinehealthcheck`</a> | Issues or PRs related to machinehealthchecks| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/machinepool" href="#area/machinepool">`area/machinepool`</a> | Issues or PRs related to machinepools| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/machineset" href="#area/machineset">`area/machineset`</a> | Issues or PRs related to machinesets| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/metrics" href="#area/metrics">`area/metrics`</a> | Issues or PRs related to metrics| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/networking" href="#area/networking">`area/networking`</a> | Issues or PRs related to networking| label | |
| <a id="area/provider/bootstrap-kubeadm" href="#area/provider/bootstrap-kubeadm">`area/provider/bootstrap-kubeadm`</a> | Issues or PRs related to CAPBK| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/provider/control-plane-kubeadm" href="#area/provider/control-plane-kubeadm">`area/provider/control-plane-kubeadm`</a> | Issues or PRs related to KCP| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/provider/core" href="#area/provider/core">`area/provider/core`</a> | Issues or PRs related to the core provider| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/provider/infrastructure-docker" href="#area/provider/infrastructure-docker">`area/provider/infrastructure-docker`</a> | Issues or PRs related to the docker infrastructure provider| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/provider/infrastructure-in-memory" href="#area/provider/infrastructure-in-memory">`area/provider/infrastructure-in-memory`</a> | Issues or PRs related to the in-memory infrastructure provider| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/release" href="#area/release">`area/release`</a> | Issues or PRs related to releasing| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/runtime-sdk" href="#area/runtime-sdk">`area/runtime-sdk`</a> | Issues or PRs related to Runtime SDK| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/security" href="#area/security">`area/security`</a> | Issues or PRs related to security| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/testing" href="#area/testing">`area/testing`</a> | Issues or PRs related to testing| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/upgrades" href="#area/upgrades">`area/upgrades`</a> | Issues or PRs related to upgrades| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/util" href="#area/util">`area/util`</a> | Issues or PRs related to utils| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/design" href="#kind/design">`kind/design`</a> | Categorizes issue or PR as related to design.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/proposal" href="#kind/proposal">`kind/proposal`</a> | Issues or PRs related to proposals.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/release-blocking" href="#kind/release-blocking">`kind/release-blocking`</a> | Issues or PRs that need to be closed before the next CAPI release| approvers |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes-sigs/cluster-api, only for PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="do-not-merge/needs-area" href="#do-not-merge/needs-area">`do-not-merge/needs-area`</a> | PR is missing an area label| prow |  [require-matching-label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/require-matching-label) |

## Labels that apply to kubernetes-sigs/cluster-api-operator, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/api" href="#area/api">`area/api`</a> | Issues or PRs related to the APIs| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/artifacts" href="#area/artifacts">`area/artifacts`</a> | Issues or PRs related to the hosting of release artifacts| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/ci" href="#area/ci">`area/ci`</a> | Issues or PRs related to ci| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/release" href="#area/release">`area/release`</a> | Issues or PRs related to releasing| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/security" href="#area/security">`area/security`</a> | Issues or PRs related to security| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/testing" href="#area/testing">`area/testing`</a> | Issues or PRs related to testing| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/upgrades" href="#area/upgrades">`area/upgrades`</a> | Issues or PRs related to upgrades| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/ux" href="#area/ux">`area/ux`</a> | Issues or PRs related to UX| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/design" href="#kind/design">`kind/design`</a> | Categorizes issue or PR as related to design.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/proposal" href="#kind/proposal">`kind/proposal`</a> | Issues or PRs related to proposals.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/release-blocking" href="#kind/release-blocking">`kind/release-blocking`</a> | Issues or PRs that need to be closed before the next CAPI Operator release| approvers |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes-sigs/cluster-api-provider-aws, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="adr-required" href="#adr-required">`adr-required`</a> | Denotes an issue or PR contains a decision that needs documenting using an ADR.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/api" href="#area/api">`area/api`</a> | Issues or PRs related to the APIs| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/artifacts" href="#area/artifacts">`area/artifacts`</a> | Issues or PRs related to the hosting of release artifacts| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/clusterawsadm" href="#area/clusterawsadm">`area/clusterawsadm`</a> | Issues or PRs related to clusterawsadm| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/code-organization" href="#area/code-organization">`area/code-organization`</a> | Issues or PRs related to Cluster API code organization| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/conformance" href="#area/conformance">`area/conformance`</a> | Issues or PRs related to Cluster API and Kubernetes conformance tests| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/deflake" href="#area/deflake">`area/deflake`</a> | Issues or PRs related to deflaking Cluster API tests| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/kubetest" href="#area/kubetest">`area/kubetest`</a> | Issues or PRs related to Cluster API Kubetest2 Provider| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/networking" href="#area/networking">`area/networking`</a> | Issues or PRs related to networking| label | |
| <a id="area/provider/eks" href="#area/provider/eks">`area/provider/eks`</a> | Issues or PRs related to Amazon EKS provider| label | |
| <a id="area/provider/rosa" href="#area/provider/rosa">`area/provider/rosa`</a> | Issues or PRs related to Red Hat ROSA provider| label | |
| <a id="area/release" href="#area/release">`area/release`</a> | Issues or PRs related to releasing| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/security" href="#area/security">`area/security`</a> | Issues or PRs related to security| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/testing" href="#area/testing">`area/testing`</a> | Issues or PRs related to testing| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/upgrades" href="#area/upgrades">`area/upgrades`</a> | Issues or PRs related to upgrades| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/ux" href="#area/ux">`area/ux`</a> | Issues or PRs related to UX| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/vpc" href="#area/vpc">`area/vpc`</a> | Issues or PRs related to Amazon VPCs| label | |
| <a id="kind/backport" href="#kind/backport">`kind/backport`</a> | Issues or PRs requiring backports| approvers |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/proposal" href="#kind/proposal">`kind/proposal`</a> | Issues or PRs related to proposals.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/release-blocking" href="#kind/release-blocking">`kind/release-blocking`</a> | Issues or PRs that need to be closed before the next release| approvers |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes-sigs/cluster-api-provider-aws, only for issues

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/admin" href="#area/admin">`area/admin`</a> | Indicates an issue on admin area.| label | |

## Labels that apply to kubernetes-sigs/cluster-api-provider-azure, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="kind/proposal" href="#kind/proposal">`kind/proposal`</a> | Issues or PRs related to proposals.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes-sigs/cluster-api-provider-gcp, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/gke" href="#area/gke">`area/gke`</a> | Issues or PRs related to GKE| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes-sigs/cluster-api-provider-vsphere, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/govmomi" href="#area/govmomi">`area/govmomi`</a> | Issues or PRs related to the govmomi mode| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/supervisor" href="#area/supervisor">`area/supervisor`</a> | Issues or PRs related to the supervisor mode| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes-sigs/contributor-tweets, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/contributor-comms" href="#area/contributor-comms">`area/contributor-comms`</a> | Issues or PRs related to the upstream marketing team| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/tweet" href="#kind/tweet">`kind/tweet`</a> | Issues to parse and tweet from K8sContributor handle| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes-sigs/controller-runtime, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="kind/design" href="#kind/design">`kind/design`</a> | Categorizes issue or PR as related to design.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes-sigs/gateway-api, only for issues

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="kind/user-story" href="#kind/user-story">`kind/user-story`</a> | Categorizes an issue as capturing a user story| humans | |

## Labels that apply to kubernetes-sigs/gateway-api, only for PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="kind/gep" href="#kind/gep">`kind/gep`</a> | PRs related to Gateway Enhancement Proposal(GEP)| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes-sigs/kind, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/provider/docker" href="#area/provider/docker">`area/provider/docker`</a> | Issues or PRs related to docker| humans | |
| <a id="area/provider/podman" href="#area/provider/podman">`area/provider/podman`</a> | Issues or PRs related to podman| humans | |

## Labels that apply to kubernetes-sigs/krew, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="priority/P0" href="#priority/P0">`priority/P0`</a> | P0 issues or PRs| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="priority/P1" href="#priority/P1">`priority/P1`</a> | P1 issues or PRs| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="priority/P2" href="#priority/P2">`priority/P2`</a> | P2 issues or PRs| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="priority/P3" href="#priority/P3">`priority/P3`</a> | P3 issues or PRs| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes-sigs/kubespray, only for PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="ci-extended" href="#ci-extended">`ci-extended`</a> | Run additional tests| approvers or reviewers or members |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="ci-full" href="#ci-full">`ci-full`</a> | Run every available tests| approvers or reviewers or members |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="ci-short" href="#ci-short">`ci-short`</a> | Run a quick CI pipeline| approvers or reviewers or members |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes-sigs/promo-tools, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/artifacts" href="#area/artifacts">`area/artifacts`</a> | Issues or PRs related to the hosting of release artifacts for subprojects| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/release-eng" href="#area/release-eng">`area/release-eng`</a> | Issues or PRs related to the Release Engineering subproject <br><br> This was previously `area/release-infra`, | label | |

## Labels that apply to kubernetes/community, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/annual-reports" href="#area/annual-reports">`area/annual-reports`</a> | Issues or PRs related to the annual reports| label | |
| <a id="area/cn-summit" href="#area/cn-summit">`area/cn-summit`</a> | Issues or PRs related to the Contributor Summit in China| label | |
| <a id="area/code-organization" href="#area/code-organization">`area/code-organization`</a> | Issues or PRs related to kubernetes code organization| label | |
| <a id="area/conformance" href="#area/conformance">`area/conformance`</a> | Issues or PRs related to kubernetes conformance tests| label | |
| <a id="area/contributor-comms" href="#area/contributor-comms">`area/contributor-comms`</a> | Issues or PRs related to the upstream marketing team| label | |
| <a id="area/contributor-guide" href="#area/contributor-guide">`area/contributor-guide`</a> | Issues or PRs related to the contributor guide| label | |
| <a id="area/contributor-summit" href="#area/contributor-summit">`area/contributor-summit`</a> | Issues or PRs related to all Contributor Summit events| label | |
| <a id="area/deflake" href="#area/deflake">`area/deflake`</a> | Issues or PRs related to deflaking kubernetes tests| label | |
| <a id="area/developer-guide" href="#area/developer-guide">`area/developer-guide`</a> | Issues or PRs related to the developer guide| label | |
| <a id="area/devstats" href="#area/devstats">`area/devstats`</a> | Issues or PRs related to the devstats subproject| label | |
| <a id="area/e2e-test-framework" href="#area/e2e-test-framework">`area/e2e-test-framework`</a> | Issues or PRs related to refactoring the kubernetes e2e test framework| label | |
| <a id="area/elections" href="#area/elections">`area/elections`</a> | Issues or PRs related to community elections| label | |
| <a id="area/enhancements" href="#area/enhancements">`area/enhancements`</a> | Issues or PRs related to the Enhancements subproject| label | |
| <a id="area/eu-summit" href="#area/eu-summit">`area/eu-summit`</a> | Issues or PRs related to the Contributor Summit in Europe| label | |
| <a id="area/github-management" href="#area/github-management">`area/github-management`</a> | Issues or PRs related to GitHub Management subproject| label | |
| <a id="area/na-summit" href="#area/na-summit">`area/na-summit`</a> | Issues or PRs related to the Contributor Summit in North America| label | |
| <a id="area/release-team" href="#area/release-team">`area/release-team`</a> | Issues or PRs related to the release-team subproject| label | |
| <a id="area/slack-management" href="#area/slack-management">`area/slack-management`</a> | Issues or PRs related to the Slack Management subproject| label | |

## Labels that apply to kubernetes/dns, only for issues

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/kubedns" href="#area/kubedns">`area/kubedns`</a> | Issues related to kube-dns| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/nodelocaldns" href="#area/nodelocaldns">`area/nodelocaldns`</a> | Issues related to node-local-dns| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes/enhancements, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/code-organization" href="#area/code-organization">`area/code-organization`</a> | Issues or PRs related to kubernetes code organization| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/contributor-comms" href="#area/contributor-comms">`area/contributor-comms`</a> | Issues or PRs related to the upstream marketing team| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/enhancements" href="#area/enhancements">`area/enhancements`</a> | Issues or PRs related to the Enhancements subproject| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/release-eng" href="#area/release-eng">`area/release-eng`</a> | Issues or PRs related to the Release Engineering subproject <br><br> This was previously `area/release-infra`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="kind/kep" href="#kind/kep">`kind/kep`</a> | Categorizes KEP tracking issues and PRs modifying the KEP directory| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes/enhancements, only for issues

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="lead-opted-in" href="#lead-opted-in">`lead-opted-in`</a> | Denotes that an issue has been opted in to a release| SIG leads ONLY |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="stage/alpha" href="#stage/alpha">`stage/alpha`</a> | Denotes an issue tracking an enhancement targeted for Alpha status| anyone |  [stage](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/stage) |
| <a id="stage/beta" href="#stage/beta">`stage/beta`</a> | Denotes an issue tracking an enhancement targeted for Beta status| anyone |  [stage](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/stage) |
| <a id="stage/stable" href="#stage/stable">`stage/stable`</a> | Denotes an issue tracking an enhancement targeted for Stable/GA status| anyone |  [stage](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/stage) |
| <a id="tracked/no" href="#tracked/no">`tracked/no`</a> | Denotes an enhancement issue is NOT actively being tracked by the Release Team| Release enhancements team ONLY |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="tracked/out-of-tree" href="#tracked/out-of-tree">`tracked/out-of-tree`</a> | Denotes an out-of-tree enhancement issue, which does not need to be tracked by the Release Team| Release enhancements team ONLY |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="tracked/yes" href="#tracked/yes">`tracked/yes`</a> | Denotes an enhancement issue is actively being tracked by the Release Team| Release enhancements team ONLY |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes/k8s.io, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/access" href="#area/access">`area/access`</a> | Define who has access to what via IAM bindings, role bindings, policy, etc.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/apps" href="#area/apps">`area/apps`</a> | Application management, code in apps/ <br><br> This was previously `area/cluster-infra`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/apps/cert-manager" href="#area/apps/cert-manager">`area/apps/cert-manager`</a> | cert-manager, code in apps/cert-manager/ <br><br> This was previously `area/infra/cert-manager`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/apps/gcsweb" href="#area/apps/gcsweb">`area/apps/gcsweb`</a> | gcsweb.k8s.io, code in apps/gcsweb/| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/apps/k8s-io" href="#area/apps/k8s-io">`area/apps/k8s-io`</a> | k8s.io redirector, code in apps/k8s-io/| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/apps/kubernetes-external-secrets" href="#area/apps/kubernetes-external-secrets">`area/apps/kubernetes-external-secrets`</a> | kubernetes-external-secrets, code in apps/kubernetes-external-secrets/| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/apps/perfdash" href="#area/apps/perfdash">`area/apps/perfdash`</a> | perfdash.k8s.io, code in apps/perdash/| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/apps/prow" href="#area/apps/prow">`area/apps/prow`</a> | k8s-infra-prow.k8s.io, code in apps/prow/| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/apps/publishing-bot" href="#area/apps/publishing-bot">`area/apps/publishing-bot`</a> | publishing-bot, code in apps/publishing-bot/ <br><br> This was previously `area/infra/publishing-bot`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/apps/slack-infra" href="#area/apps/slack-infra">`area/apps/slack-infra`</a> | slack.k8s.io, slack-infra, code in apps/slack-infra| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/artifacts" href="#area/artifacts">`area/artifacts`</a> | Issues or PRs related to the hosting of release artifacts for subprojects| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/audit" href="#area/audit">`area/audit`</a> | Audit of project resources, audit followup issues, code in audit/ <br><br> This was previously `area/infra/auditing`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/bash" href="#area/bash">`area/bash`</a> | Bash scripts, testing them, writing less of them, code in infra/gcp/| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/billing" href="#area/billing">`area/billing`</a> | Issues or PRs related to billing| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/cluster-mgmt" href="#area/cluster-mgmt">`area/cluster-mgmt`</a> | REMOVING. This will be deleted after 2021-08-04 00:00:00 +0000 UTC <br><br> Issues or PRs related to managing k8s clusters to run k8s-infra| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/dns" href="#area/dns">`area/dns`</a> | DNS records for k8s.io, kubernetes.io, k8s.dev, etc., code in dns/| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/groups" href="#area/groups">`area/groups`</a> | Google Groups management, code in groups/| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/infra" href="#area/infra">`area/infra`</a> | Infrastructure management, infrastructure design, code in infra/ <br><br> This was previously `area/cluster-infra`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/infra/aws" href="#area/infra/aws">`area/infra/aws`</a> | Issues or PRs related to Kubernetes AWS infrastructure| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/infra/azure" href="#area/infra/azure">`area/infra/azure`</a> | Issues or PRs related to Kubernetes Azure infrastructure| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/infra/gcp" href="#area/infra/gcp">`area/infra/gcp`</a> | Issues or PRs related to Kubernetes GCP infrastructure| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/infra/monitoring" href="#area/infra/monitoring">`area/infra/monitoring`</a> | REMOVING. This will be deleted after 2021-08-04 00:00:00 +0000 UTC <br><br> Issues or PRs related to monitoring of the Kubernetes project infrastructure| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/infra/reliability" href="#area/infra/reliability">`area/infra/reliability`</a> | REMOVING. This will be deleted after 2021-08-04 00:00:00 +0000 UTC <br><br> Issues or PR related to reliability of the Kubernetes project infrastructure| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/k8s.gcr.io" href="#area/k8s.gcr.io">`area/k8s.gcr.io`</a> | Code in k8s.gcr.io/| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/policy" href="#area/policy">`area/policy`</a> | Crafting policy, policy decisions, code in policy/| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/prow" href="#area/prow">`area/prow`</a> | Setting up or working with prow in general, prow.k8s.io, prow build clusters| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/registry.k8s.io" href="#area/registry.k8s.io">`area/registry.k8s.io`</a> | Code in registry.k8s.io/| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/release-eng" href="#area/release-eng">`area/release-eng`</a> | Issues or PRs related to the Release Engineering subproject| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/terraform" href="#area/terraform">`area/terraform`</a> | Terraform modules, testing them, writing more of them, code in infra/gcp/clusters/ <br><br> This was previously `area/infra/terraform`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes/k8s.io, only for PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="skip-review" href="#skip-review">`skip-review`</a> | Indicates a PR is trusted, used by tide for auto-merging PRs.| autobump bot | |

## Labels that apply to kubernetes/kubeadm, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="kind/design" href="#kind/design">`kind/design`</a> | Categorizes issue or PR as related to design.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes/kubernetes, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/artifacts" href="#area/artifacts">`area/artifacts`</a> | Issues or PRs related to the hosting of release artifacts for subprojects| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/code-organization" href="#area/code-organization">`area/code-organization`</a> | Issues or PRs related to kubernetes code organization| label | |
| <a id="area/conformance" href="#area/conformance">`area/conformance`</a> | Issues or PRs related to kubernetes conformance tests| label | |
| <a id="area/deflake" href="#area/deflake">`area/deflake`</a> | Issues or PRs related to deflaking kubernetes tests| label | |
| <a id="area/e2e-test-framework" href="#area/e2e-test-framework">`area/e2e-test-framework`</a> | Issues or PRs related to refactoring the kubernetes e2e test framework| label | |
| <a id="area/network-policy" href="#area/network-policy">`area/network-policy`</a> | Issues or PRs related to Network Policy subproject| label | |
| <a id="area/node-lifecycle" href="#area/node-lifecycle">`area/node-lifecycle`</a> | Issues or PRs related to Node lifecycle| label | |
| <a id="area/pod-lifecycle" href="#area/pod-lifecycle">`area/pod-lifecycle`</a> | Issues or PRs related to Pod lifecycle| label | |
| <a id="area/release-eng" href="#area/release-eng">`area/release-eng`</a> | Issues or PRs related to the Release Engineering subproject <br><br> This was previously `area/release-infra`, | label | |
| <a id="area/stable-metrics" href="#area/stable-metrics">`area/stable-metrics`</a> | Issues or PRs involving stable metrics| label | |
| <a id="official-cve-feed" href="#official-cve-feed">`official-cve-feed`</a> | Issues or PRs related to CVEs officially announced by Security Response Committee (SRC)| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes/kubernetes, only for issues

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/admin" href="#area/admin">`area/admin`</a> | Indicates an issue on admin area.| label | |
| <a id="area/api" href="#area/api">`area/api`</a> | Indicates an issue on api area.| label | |

## Labels that apply to kubernetes/kubernetes, only for PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="do-not-merge/contains-merge-commits" href="#do-not-merge/contains-merge-commits">`do-not-merge/contains-merge-commits`</a> | Indicates a PR which contains merge commits.| prow |  [mergecommitblocker](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/mergecommitblocker) |
| <a id="do-not-merge/needs-kind" href="#do-not-merge/needs-kind">`do-not-merge/needs-kind`</a> | Indicates a PR lacks a `kind/foo` label and requires one.| prow |  [require-matching-label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/require-matching-label) |
| <a id="do-not-merge/needs-sig" href="#do-not-merge/needs-sig">`do-not-merge/needs-sig`</a> | Indicates an issue or PR lacks a `sig/foo` label and requires one.| prow |  [require-matching-label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/require-matching-label) |
| <a id="needs-priority" href="#needs-priority">`needs-priority`</a> | Indicates a PR lacks a `priority/foo` label and requires one.| prow |  [require-matching-label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/require-matching-label) |

## Labels that apply to kubernetes/org, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/github-management" href="#area/github-management">`area/github-management`</a> | Issues or PRs related to GitHub Management subproject| label | |

## Labels that apply to kubernetes/org, only for issues

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/github-integration" href="#area/github-integration">`area/github-integration`</a> | Third-party integrations, webhooks, or GitHub Apps| label | |
| <a id="area/github-membership" href="#area/github-membership">`area/github-membership`</a> | Requesting membership in a Kubernetes GitHub Organization or Team| label | |
| <a id="area/github-repo" href="#area/github-repo">`area/github-repo`</a> | Creating, migrating or deleting a Kubernetes GitHub Repository| label | |

## Labels that apply to kubernetes/release, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/artifacts" href="#area/artifacts">`area/artifacts`</a> | Issues or PRs related to the hosting of release artifacts for subprojects| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/release-eng" href="#area/release-eng">`area/release-eng`</a> | Issues or PRs related to the Release Engineering subproject <br><br> This was previously `area/release-infra`, | label | |
| <a id="area/release-eng/security" href="#area/release-eng/security">`area/release-eng/security`</a> | Issues or PRs related to release engineering security| label | |

## Labels that apply to kubernetes/sig-release, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/artifacts" href="#area/artifacts">`area/artifacts`</a> | Issues or PRs related to the hosting of release artifacts for subprojects| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/enhancements" href="#area/enhancements">`area/enhancements`</a> | Issues or PRs related to the Enhancements subproject| label | |
| <a id="area/release-eng" href="#area/release-eng">`area/release-eng`</a> | Issues or PRs related to the Release Engineering subproject <br><br> This was previously `area/release-infra`, | label | |
| <a id="area/release-eng/security" href="#area/release-eng/security">`area/release-eng/security`</a> | Issues or PRs related to release engineering security| label | |
| <a id="area/release-team" href="#area/release-team">`area/release-team`</a> | Issues or PRs related to the release-team subproject| label | |

## Labels that apply to kubernetes/sig-security, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/security-assessment" href="#area/security-assessment">`area/security-assessment`</a> | Issues or PRs related to security assessment of sub-projects| anyone | |

## Labels that apply to kubernetes/test-infra, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/artifacts" href="#area/artifacts">`area/artifacts`</a> | Issues or PRs related to the hosting of release artifacts for subprojects| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/boskos" href="#area/boskos">`area/boskos`</a> | Issues or PRs related to code in /boskos| label | |
| <a id="area/code-organization" href="#area/code-organization">`area/code-organization`</a> | Issues or PRs related to kubernetes code organization| label | |
| <a id="area/config" href="#area/config">`area/config`</a> | Issues or PRs related to code in /config| label | |
| <a id="area/conformance" href="#area/conformance">`area/conformance`</a> | Issues or PRs related to kubernetes conformance tests| label | |
| <a id="area/deflake" href="#area/deflake">`area/deflake`</a> | Issues or PRs related to deflaking kubernetes tests| label | |
| <a id="area/e2e-test-framework" href="#area/e2e-test-framework">`area/e2e-test-framework`</a> | Issues or PRs related to refactoring the kubernetes e2e test framework| label | |
| <a id="area/enhancements" href="#area/enhancements">`area/enhancements`</a> | Issues or PRs related to the Enhancements subproject| label | |
| <a id="area/ghproxy" href="#area/ghproxy">`area/ghproxy`</a> | Issues or PRs related to code in /ghproxy| label | |
| <a id="area/github-management" href="#area/github-management">`area/github-management`</a> | Issues or PRs related to GitHub Management subproject| label | |
| <a id="area/gopherage" href="#area/gopherage">`area/gopherage`</a> | Issues or PRs related to code in /gopherage| humans | |
| <a id="area/greenhouse" href="#area/greenhouse">`area/greenhouse`</a> | Issues or PRs related to code in /greenhouse (our remote bazel cache)| label | |
| <a id="area/gubernator" href="#area/gubernator">`area/gubernator`</a> | Issues or PRs related to code in /gubernator| label | |
| <a id="area/kind" href="#area/kind">`area/kind`</a> | Issues or PRs related to code in /kind| label | |
| <a id="area/label_sync" href="#area/label_sync">`area/label_sync`</a> | Issues or PRs related to code in /label_sync| label | |
| <a id="area/mungegithub" href="#area/mungegithub">`area/mungegithub`</a> | Issues or PRs related to code in /mungegithub| label | |
| <a id="area/planter" href="#area/planter">`area/planter`</a> | Issues or PRs related to code in /planter| label | |
| <a id="area/prow" href="#area/prow">`area/prow`</a> | Issues or PRs related to prow| label | |
| <a id="area/prow/branchprotector" href="#area/prow/branchprotector">`area/prow/branchprotector`</a> | Issues or PRs related to prow's branchprotector component| label | |
| <a id="area/prow/bump" href="#area/prow/bump">`area/prow/bump`</a> | Updates to the k8s prow cluster| label | |
| <a id="area/prow/clonerefs" href="#area/prow/clonerefs">`area/prow/clonerefs`</a> | Issues or PRs related to prow's clonerefs component| label | |
| <a id="area/prow/config-bootstrapper" href="#area/prow/config-bootstrapper">`area/prow/config-bootstrapper`</a> | Issues or PRs related to prow's config-bootstrapper utility| label | |
| <a id="area/prow/crier" href="#area/prow/crier">`area/prow/crier`</a> | Issues or PRs related to prow's crier component| label | |
| <a id="area/prow/deck" href="#area/prow/deck">`area/prow/deck`</a> | Issues or PRs related to prow's deck component| label | |
| <a id="area/prow/entrypoint" href="#area/prow/entrypoint">`area/prow/entrypoint`</a> | Issues or PRs related to prow's entrypoint component| label | |
| <a id="area/prow/gcsupload" href="#area/prow/gcsupload">`area/prow/gcsupload`</a> | Issues or PRs related to prow's gcsupload component| label | |
| <a id="area/prow/gerrit" href="#area/prow/gerrit">`area/prow/gerrit`</a> | Issues or PRs related to prow's gerrit component| label | |
| <a id="area/prow/hook" href="#area/prow/hook">`area/prow/hook`</a> | Issues or PRs related to prow's hook component| label | |
| <a id="area/prow/horologium" href="#area/prow/horologium">`area/prow/horologium`</a> | Issues or PRs related to prow's horologium component| label | |
| <a id="area/prow/initupload" href="#area/prow/initupload">`area/prow/initupload`</a> | Issues or PRs related to prow's initupload component| label | |
| <a id="area/prow/jenkins-operator" href="#area/prow/jenkins-operator">`area/prow/jenkins-operator`</a> | Issues or PRs related to prow's jenkins-operator component| label | |
| <a id="area/prow/knative-build" href="#area/prow/knative-build">`area/prow/knative-build`</a> | Issues or PRs related to prow's knative-build controller component| label | |
| <a id="area/prow/mkpj" href="#area/prow/mkpj">`area/prow/mkpj`</a> | Issues or PRs related to prow's mkpj component| label | |
| <a id="area/prow/mkpod" href="#area/prow/mkpod">`area/prow/mkpod`</a> | Issues or PRs related to prow's mkpod component| label | |
| <a id="area/prow/peribolos" href="#area/prow/peribolos">`area/prow/peribolos`</a> | Issues or PRs related to prow's peribolos component| label | |
| <a id="area/prow/phony" href="#area/prow/phony">`area/prow/phony`</a> | Issues or PRs related to prow's phony component| label | |
| <a id="area/prow/plank" href="#area/prow/plank">`area/prow/plank`</a> | Issues or PRs related to prow's plank component| label | |
| <a id="area/prow/plugins" href="#area/prow/plugins">`area/prow/plugins`</a> | Issues or PRs related to prow's plugins for the hook component| label | |
| <a id="area/prow/pod-utilities" href="#area/prow/pod-utilities">`area/prow/pod-utilities`</a> | Issues or PRs related to prow's pod-utilities component| label | |
| <a id="area/prow/pubsub" href="#area/prow/pubsub">`area/prow/pubsub`</a> | Issues or PRs related to prow's pubsub reporter component| label | |
| <a id="area/prow/sidecar" href="#area/prow/sidecar">`area/prow/sidecar`</a> | Issues or PRs related to prow's sidecar component| label | |
| <a id="area/prow/sinker" href="#area/prow/sinker">`area/prow/sinker`</a> | Issues or PRs related to prow's sinker component| label | |
| <a id="area/prow/splice" href="#area/prow/splice">`area/prow/splice`</a> | Issues or PRs related to prow's splice component| label | |
| <a id="area/prow/spyglass" href="#area/prow/spyglass">`area/prow/spyglass`</a> | Issues or PRs related to prow's spyglass UI| label | |
| <a id="area/prow/status-reconciler" href="#area/prow/status-reconciler">`area/prow/status-reconciler`</a> | Issues or PRs related to reconciling status when jobs change| label | |
| <a id="area/prow/tide" href="#area/prow/tide">`area/prow/tide`</a> | Issues or PRs related to prow's tide component| label | |
| <a id="area/prow/tot" href="#area/prow/tot">`area/prow/tot`</a> | Issues or PRs related to prow's tot component| label | |
| <a id="area/release-eng" href="#area/release-eng">`area/release-eng`</a> | Issues or PRs related to the Release Engineering subproject <br><br> This was previously `area/release-infra`, | label | |
| <a id="area/robots" href="#area/robots">`area/robots`</a> | Issues or PRs related to code in /robots| label | |
| <a id="kind/oncall-hotlist" href="#kind/oncall-hotlist">`kind/oncall-hotlist`</a> | Categorizes issue or PR as tracked by test-infra oncall.| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes/test-infra, only for PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="skip-review" href="#skip-review">`skip-review`</a> | Indicates a PR is trusted, used by tide for auto-merging PRs.| autobump bot | |

## Labels that apply to kubernetes/website, for both issues and PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="area/blog" href="#area/blog">`area/blog`</a> | Issues or PRs related to the Kubernetes Blog subproject| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/contributor-comms" href="#area/contributor-comms">`area/contributor-comms`</a> | Issues or PRs related to the upstream marketing team| humans | |
| <a id="area/release-eng" href="#area/release-eng">`area/release-eng`</a> | Issues or PRs related to the Release Engineering subproject| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="area/web-development" href="#area/web-development">`area/web-development`</a> | Issues or PRs related to the kubernetes.io's infrastructure, design, or build processes| humans | |
| <a id="language/ar" href="#language/ar">`language/ar`</a> | Issues or PRs related to Arabic language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/bn" href="#language/bn">`language/bn`</a> | Issues or PRs related to Bengali language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/de" href="#language/de">`language/de`</a> | Issues or PRs related to German language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/en" href="#language/en">`language/en`</a> | Issues or PRs related to English language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/es" href="#language/es">`language/es`</a> | Issues or PRs related to Spanish language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/fa" href="#language/fa">`language/fa`</a> | Issues or PRs related to Persian language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/fr" href="#language/fr">`language/fr`</a> | Issues or PRs related to French language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/hi" href="#language/hi">`language/hi`</a> | Issues or PRs related to Hindi language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/id" href="#language/id">`language/id`</a> | Issues or PRs related to Indonesian language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/it" href="#language/it">`language/it`</a> | Issues or PRs related to Italian language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/ja" href="#language/ja">`language/ja`</a> | Issues or PRs related to Japanese language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/ko" href="#language/ko">`language/ko`</a> | Issues or PRs related to Korean language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/no" href="#language/no">`language/no`</a> | Issues or PRs related to Norwegian language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/pl" href="#language/pl">`language/pl`</a> | Issues or PRs related to Polish language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/pt" href="#language/pt">`language/pt`</a> | Issues or PRs related to Portuguese language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/ru" href="#language/ru">`language/ru`</a> | Issues or PRs related to Russian language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/uk" href="#language/uk">`language/uk`</a> | Issues or PRs related to Ukrainian language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/vi" href="#language/vi">`language/vi`</a> | Issues or PRs related to Vietnamese language| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |
| <a id="language/zh" href="#language/zh">`language/zh`</a> | Issues or PRs related to Chinese language <br><br> This was previously `language/cn`, | anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes/website, only for issues

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="team/katacoda" href="#team/katacoda">`team/katacoda`</a> | Issues with the Katacoda infrastructure for tutorials| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |

## Labels that apply to kubernetes/website, only for PRs

| Name | Description | Added By | Prow Plugin |
| ---- | ----------- | -------- | --- |
| <a id="refactor" href="#refactor">`refactor`</a> | Indicates a PR with large refactoring changes e.g. removes files or moves content| anyone |  [label](https://github.com/kubernetes-sigs/prow/tree/main/pkg/plugins/label) |


