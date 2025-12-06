# SIG Node CI Test Coverage Guidance

## Test Tabs

#### 1. **sig-node-release-informing**

This test tab should be checked in the weeks prior to code freeze and release,
and signals general stability. 

<table>
  <tr>
   <td>
<strong>Variables</strong>
   </td>
   <td><strong>Values</strong>
   </td>
  </tr>
  <tr>
   <td>cri
   </td>
   <td>containerd,  CRI-O
   </td>
  </tr>
   <td>os
   </td>
   <td>COS, ubuntu
   </td>
  </tr>
  <tr>
   <td>release-branch
   </td>
   <td>master
   </td>
  </tr>
  <tr>
   <td>test-type
   </td>
   <td>node-conformance, serial, standalone, standalone-serial, flaky
   </td>
  </tr>
</table>

**Test Names**

(pull|ci)-node-{cri}-{os}-{release-branch}-{test-type}

For node-conformance, serial, and standalone tests will also have the following jobs:
* (pull|ci)-node-{cri}-{os}-{release-branch}-{test-type}-beta-enabled
  * All beta features will be **enabled**, but corresponding beta tests will not run.
* (pull|ci)-node-{cri}-{os}-{release-branch}-{test-type}-alpha-and-beta-enabled
  * All alpha and beta features will be **enabled**, but corresponding alpha and beta tests will not run.

**Test Args**

* node-conformance
  * Focus: `NodeConformance`
  * Skip: `Flaky`, `Slow`, `Serial`
* serial
  * Focus: `Serial`
  * Skip: `Flaky`, `Benchmark`, `Feature:.+`, `FeatureGate:.+`
* standalone
  * Focus: `Feature:StandaloneMode`
  * Skip: `Flaky`, `Serial`, `Feature:.+`, `FeatureGate:.+`
* standalone-serial
  * Focus: `Feature:StandaloneMode`, `Serial`
  * Skip: `Flaky`, `Feature:.+`, `FeatureGate:.+`
* flaky
  * Focus: `Flaky`
  * Skip: `Benchmark`, `Feature:.+`, `FeatureGate:.+`

#### 2. **sig-node-node-conformance**

This tab is is focused on node conformance tests. 

**Test Names**

This tab will include all of the **node-conformance** tests from the previously described **sig-node-release-informing** tab,
and will additionally have the following jobs:

<table>
  <tr>
   <td>
<strong>Variables</strong>
   </td>
   <td><strong>Values</strong>
   </td>
  </tr>
  <tr>
   <td>cri
   </td>
   <td>containerd
   </td>
  <tr>
   <td>os
   </td>
   <td>COS
   </td>
  </tr>
  <tr>
   <td>test-type
   </td>
   <td>node-conformance
   </td>
  </tr>
    <tr>
   <td>release-branch
   </td>
   <td>master, release-[N], release[N-1], release[N-2], release[N-3]
   </td>
  </tr>
</table>

**Test Names**

* (pull|ci)-node-{cri}-{os}-{release-branch}-{test-type}
* (pull|ci)-node-{cri}-{os}-{release-branch}-{test-type}-beta-enabled
  * All beta features will be **enabled**, but corresponding beta tests will not run.
* (pull|ci)-node-{cri}-{os}-{release-branch}-{test-type}-alpha-and-beta-enabled
  * All alpha and beta features will be **enabled**, but corresponding alpha and beta tests will not run.

**Test Args**

* Focus: `NodeConformance`
* Skip: `Flaky`, `Slow`, `Serial`

**Release responsibility for new version (with k8s release cycle)**
* Add a new image for a new K8s version.
    * Ensure COS and Ubuntu versions are set to K8s X default OS combination same as supported by managed kubernetes distributions AKS, EKS, GKE, etc. We can start with GKE supported combinations for some coverage, and then add for other distros when needed.
* Update the release branches for the tests. Add tests against the new release branch, and remove tests against N-3 branch.

#### 3. **sig-node-features**

This test tab covers special features. These fall into two categories:

1. Tests covering alpha and beta features that need only the feature-gate to
be enabled.

2. Tests that require special setup:
    - Special node setup, like swap, eviction, device plugin, huge pages, etc.
    - Special K8s configuration, like Kubelet Credential Provider, lock contention, etc.
    - Tests that are specific to a cloud provider, e.g. GCP Credential Provider or EC2 tests.

**Feature-gated tests**

For tests that need only the feature-gate to be enabled, the following naming conventions are used:

* (pull|ci)-node-containerd-cos-master-beta-features
  * All beta features will be **enabled**.
  * All tests guarded by a beta feature gate will run.
    * Focus: `FeatureGate:.+`, `Beta`
    * Skip: `Alpha`, `Flaky`, `Slow`, `Serial`, `Feature:.+`
* (pull|ci)-node-containerd-cos-master-beta-features-serial
  * All beta features will be **enabled**.
  * All tests guarded by a beta feature gate will run.
    * Focus: `FeatureGate:.+`, `Beta`, `Serial`
    * Skip: `Alpha`, `Flaky`, `Slow`, `Feature:.+`
* (pull|ci)-node-containerd-cos-master-alpha-and-beta-features
  * All beta features will be **enabled**.
  * All tests guarded by a beta feature gate will run.
    * Focus: `FeatureGate:.+`, `Alpha` | `Beta`
    * Skip: `Flaky`, `Slow`, `Serial`, `Feature:.+`
* (pull|ci)-node-containerd-cos-master-alpha-and-beta-features-serial
  * All beta features will be **enabled**.
  * All tests guarded by a beta feature gate will run.
    * Focus: `FeatureGate:.+`, `Alpha` | `Beta`, `Serial`
    * Skip: `Flaky`, `Slow`, `Feature:.+`

**Tests that require special setup**

Tests that require special setup should:
* Have a unique name to identify the feature.
* Follow (pull|ci)-node-{cri}-{os}-{release-branch}-{test-type|feature-name} for ordering the variables for non-default values in test names

For example, a job for crio-specific serial tests for swap on a fedora OS should be named `ci-node-crio-fedora-master-swap-serial`.

#### 4. **sig-node-containerd**

This test tab is for the following test types:

* Node Features against containerd main, and for combinations of K8s X containerd versions supported by managed kubernetes distributions AKS, EKS, GKE, etc. We can start with GKE supported combinations for some coverage, and then add for other distros when needed.
* Node Conformance against containerd main, and for combinations of K8s X containerd versions supported by managed kubernetes distributions.
* Serial against containerd main, and for combinations of K8s X containerd versions supported by managed kubernetes distributions.
* Containerd e2e tests for all containerd versions.
* Containerd Build tests for all containerd versions.

#### 5. **sig-node-version-skew**

This test tab is for version skew tests. These test jobs are defined by SIG Cluster Lifecycle in [this file](https://github.com/kubernetes/test-infra/blob/3caa636fe8ab79f13a7bd342dea67e966b6c1552/config/jobs/kubernetes/sig-cluster-lifecycle/kubeadm-kinder-kubelet-x-on-y.yaml).

These jobs have the name format `kubeadm-kinder-kubelet-{kubelet-release-branch}-on-{kubeadm-release-branch}`.

#### 6. **sig-node-unlabelled**

This tab is for tests that are labeled with no feature, no feature gate, nor NodeConformance. No tests are intended to be 
here, so if we see any tests here we should reevaluate accordingly. There will be two jobs in this tab as follows:

* (pull|ci)-node-containerd-cos-master-unlabelled
  * Focus: None
  * Skip: `Flaky`, `Benchmark`, `Legacy`, `Serial`, `NodeConformance`, `Conformance`, `FeatureGate:.+`, `Feature:.+`
* (pull|ci)-node-containerd-cos-master-unlabelled-serial
  * Focus: `Serial`
  * Skip: `Flaky`, `Benchmark`, `Legacy`, `NodeConformance`, `Conformance`, `FeatureGate:.+`, `Feature:.+`

Additionally, any existing jobs that do not fit cleanly into one of the other test tabs will be moved here for now,
for later reevaluation.
