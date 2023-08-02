## Signode CI Test Coverage Guidance


## Variables


<table>
  <tr>
   <td><strong>Variables</strong>
   </td>
   <td><strong>Values</strong>
   </td>
  </tr>
  <tr>
   <td>cri
   </td>
   <td>Containerd, CRI-O
   </td>
  </tr>
  <tr>
   <td>cgroup
   </td>
   <td>cgroupv1, cgroupv2
   </td>
  </tr>
  <tr>
   <td>os
   </td>
   <td>cos, ubuntu, fedora
   </td>
  </tr>
  <tr>
   <td>release branch
   </td>
   <td>N-3, N-2, N-1, N, main
   </td>
  </tr>
</table>



## Naming Convention

(pull|ci)-{$cri}-{$cgroup}-{$os}-{$test-type}-release-{$release-branch}

**Note:** Specify variable value in the name only if it isn’t set to default in a test tab.


## Test Tabs

#### 1. **sig-node-default (i.e. rename sig-node-release-blocking)**

<table>
  <tr>
   <td>
<strong>Variables</strong>
   </td>
   <td><strong>Default Values</strong>
   </td>
  </tr>
  <tr>
   <td>cri
   </td>
   <td>containerd,  CRI-O
   </td>
  </tr>
  <tr>
   <td>cgroup
   </td>
   <td>Cgroupv2
   </td>
  </tr>
  <tr>
   <td>os
   </td>
   <td>COS, ubuntu, fedora (create one instance for each OS in a single test job)
   </td>
  </tr>
  <tr>
   <td>release branch
   </td>
   <td>N-3, N-2, N-1, N, main
   </td>
  </tr>
</table>


**Test Types**
* Node Conformance
* Serial (Skip flaky, alpha and beta)
* NodeFeature (Skip flaky, alpha and beta)

**Additional tests**
* Node Conformance test for CRI-O & cgroupv1

**Test Names as per naming convention**

{cri}-{cgroup}-{os}-{test-type}-release-{release-branch}

* Node Conformance
    * node-conformance-release-(N-3)
    * node-conformance-release-(N-2)
    * node-conformance-release-(N-1)
    * node-conformance-release-(N)
    * node-conformance-release-main
* Node Feature
    * node-feature-release-(N-3)
    * node-feature-release-(N-2)
    * node-feature-release-(N-1)
    * node-feature-release-(N)
    * node-feature-release-main
* Serial
    * serial-release-(N-3)
    * serial-release-(N-2)
    * serial-release-(N-1)
    * serial-release-(N)
    * serial-release-main
* Additional Node Conformance CRI-O cgroupv1 test
    * crio-cgroupv1-node-conformance

**Release responsibility for new version (with k8s release cycle)**
* Add a new image for a new K8s version.
    * Ensure COS and Ubuntu versions are set to K8s X default OS combination same as supported by managed kubernetes distributions AKS, EKS, GKE, etc. We can start with GKE supported combinations for some coverage, and then add for other distros when needed.
* Update the release branches for the tests. Add tests against the new release branch, and remove tests against N-3 branch.

#### 2. **sig-node-kubelet**

<table>
  <tr>
   <td>
<strong>Variables</strong>
   </td>
   <td><strong>Default Values</strong>
   </td>
  </tr>
  <tr>
   <td>cri
   </td>
   <td>containerd
   </td>
  </tr>
  <tr>
   <td>cgroup
   </td>
   <td>cgroupv2
   </td>
  </tr>
  <tr>
   <td>os
   </td>
   <td>COS
   </td>
  </tr>
  <tr>
   <td>release branch
   </td>
   <td>main
   </td>
  </tr>
</table>


**Test Types**
* Node Conformance with all alpha features enabled.
* Node Conformance with all beta features enabled.
* Node Conformance with all alpha disabled.
* Node Conformance with all beta disabled
* Tests covering special features like in place pod vertical scaling. 
    * Features that require special node setup like swap, eviction, device plugin, huge pages, etc.
    * Features that require special K8s configuration e.g. Kubelet Credential Provider, lock contention, etc.
    * Feature tests specific to a cloud provider eg. GCP Credential Provider.
    * Version skew tests.

**Test Names as per naming convention**
* Unique names to identify the feature.
* Follow {cri}-{cgroup}-{os}-{test-type|feature-name}-release-{release-branch} for ordering the variables for non-default values in test names i.e. If you’d like to add a test specific to a CRI, say CRIO and cgroupv2 on Fedora OS for main branch (which is default branch for this tab, the test name would be `crio-cgroup2-fedora-feature-name`.

Examples: node-conformance-alpha-features-enabled, node-conformance-alpha-features-disabled, kubelet-credential-provider, memory-swap, version-skew-1-25-on-1-27, and so on.

**Release responsibility for new version**
* Add Version Skew tests for a new release, and remove tests for N-3 version.

#### 3. **sig-node-containerd**

**Test Types**
* Node Features against containerd main, and for combinations of K8s X containerd versions supported by managed kubernetes distributions AKS, EKS, GKE, etc. We can start with GKE supported combinations for some coverage, and then add for other distros when needed.
* Node Conformance  against containerd main, and for combinations of K8s X containerd versions supported by managed kubernetes distributions.
* Serial  against containerd main, and for combinations of K8s X containerd versions supported by managed kubernetes distributions.
* Containerd e2e tests for all containerd versions.
* Containerd Build tests for all containerd versions.