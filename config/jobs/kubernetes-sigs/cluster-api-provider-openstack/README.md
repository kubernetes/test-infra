The presubmits for the main branch are defined in
`cluster-api-provider-openstack-presubmits.yaml`.

The presubmits for each release branch are defined in
`cluster-api-provider-openstack-presubmits-<release branch>.yaml`

When creating a new release branch, copy 
`cluster-api-provider-openstack-presubmits.yaml` to a release version. For each
job in the new release version you need to modify:
* The branch name under `branches`
* The variant of the kubekins-e2e image

The main branch always uses the latest kubekins-e2e variant called
`gcr.io/k8s-staging-test-infra/kubekins-e2e:v<build>-main`. Release jobs should
be pinned to a specific variant. These are defined at
https://github.com/kubernetes/test-infra/blob/master/images/kubekins-e2e-v2/variants.yaml.
Pick a variant with the `GO_VERSION` required by the new release branch.
