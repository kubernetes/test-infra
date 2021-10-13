# Understanding Started.json and Finished.json

### Context
Prow uploads a host of artifacts throughout the life cycle of a job. Two of these artifacts that are present in each run are `started.json` and `finished.json` which contain a host of information pertaining to the job/run. These files have existed through the evolution of Kubernetes CI: from Jenkins -> Containerized Jenkins -> Bootstrap Containerized Jenkins -> Bootstrap Prow -> PodUtils. As of 2021, all jobs exist within either Bootstrap Prow or PodUtils. As the CI has evolved, so has `started/finished.json` and it's function.

Examples:
[started.json](https://storage.googleapis.com/kubernetes-jenkins/pr-logs/pull/test-infra/20825/pull-test-infra-yamllint/1359751085224366080/started.json)
[finished.json](https://storage.googleapis.com/kubernetes-jenkins/pr-logs/pull/test-infra/20825/pull-test-infra-yamllint/1359751085224366080/finished.json)

Related Issues:

1. #3412: What is the origin and purpose for the fields in these files?
2. #11100: This isn't a source of truth and prow/pod/gcs are not in sync
3. #10699: Unify *.json structures, was partially covered as part of #10703

## Format Source of Truth
There has not been a consistent source of truth for the format of these two files, which has caused issues. From discussion in the community it seems that the the [TestGrid job definition](https://github.com/GoogleCloudPlatform/testgrid/blob/master/metadata/job.go).

## Current Standards
There are currently different flavors of data format depending on if the job is Bootstrap or PodUtils. Ex of differences:
```
Bootstrapped PR (finished): "revision": "v1.20.0-alpha.0.261+06ea384605f172"
Decorated PR (finished): "revision":"5dd9241d43f256984358354d1fec468f274f9ac4"
```

[`Started.json` *Bootstrap*](https://github.com/kubernetes/test-infra/blob/a1a207e4cd847671f0a53553c664e24d26c9cdf7/jenkins/bootstrap.py#L315)
|Fields|Content|
|---|---|
|node|This is the first element in the hostname using socket.gethostname split by '.'|
|pull|The SHA linked with the 'main' repo within 'repos'|
|repo-version|"unknown" if no 'repos' otherwise read from local 'version' file (e2e tests use this path) otherwise execute version script if 'hack/lib/version.sh exists|
|timestamp|epoch time|
|repos|comes from --repos= arg|
|version|exact same as `repo-version`|
*Ex*
```
{
  "node": "0790211c-cacb-11ea-a4b9-4a19d9b965b2",
  "pull": "master:5a529aa3a0dd3a050c5302329681e871ef6c162e,93063:c25e430df7771a96c9a004d8500473a4f2ef55d3",
  "repo-version": "v1.20.0-alpha.0.261+06ea384605f172",
  "timestamp": 1595278460,
  "repos": {
    "k8s.io/kubernetes": "master:5a529aa3a0dd3a050c5302329681e871ef6c162e,93063:c25e430df7771a96c9a004d8500473a4f2ef55d3",
    "k8s.io/release": "master"
  },
  "version": "v1.20.0-alpha.0.261+06ea384605f172"
}
```

[`Finished.json` *Bootstrap*](https://github.com/kubernetes/test-infra/blob/1a958b0c2b6ddbb813bf6d23fe6b5714e9812e38/jenkins/bootstrap.py#L521)
|Fields|Content|
|---|---|
|timestamp|epoch|
|passed|bool (job success)|
|version|If version is in metadata, set from metadata same as job-version|
|result|'SUCCESS' or "FAILURE' depending on passed|
|job-version (dep)|If not existing and not 'unknown'... from metadata, try 'job-version' then 'version'|
|metadata|exact same as `repo-version`|
| metadata.repo-commit|Git rev-parse HEAD (for k8s)|
| metadata.repos|Same as started 'comes from --repo= args'|
| metadata.infra-commit|Git rev-parse HEAD (for test-infra)|
| metadata.repo|main repo for job|
| metadata.job-version| Same as job version from above|
| metadata.revision| Same as job-version|
*[Ex](https://prow.k8s.io/view/gcs/kubernetes-jenkins/pr-logs/pull/93714/pull-kubernetes-node-e2e/1291409525907132416/)*
```
{
  "timestamp": 1596732481,
  "version": "v1.20.0-alpha.0.519+e825f0a86103a6",
  "result": "SUCCESS",
  "passed": true,
  "job-version": "v1.20.0-alpha.0.519+e825f0a86103a6",
  "metadata": {
    "repo-commit": "e825f0a86103a6de00ebd20e158274c4fa625a34",
    "repos": {
      "k8s.io/kubernetes": "master:382107e6c84374b229e6188207ef026621286aa2,93714:19ff4d5a9a9b2df60019854f119e269ee035bbee"
    },
    "infra-commit": "1b7fbb373",
    "repo": "k8s.io/kubernetes",
    "job-version": "v1.20.0-alpha.0.519+e825f0a86103a6",
    "revision": "v1.20.0-alpha.0.519+e825f0a86103a6"
  }
}
```

[`Started.json` PodUtil](https://github.com/kubernetes/test-infra/blob/016edc15b8271c7528993cea0615cb11ecff201c/prow/initupload/run.go#L37)
|Fields|Content|
|---|---|
|timestamp|epoch|
|repo-version (dep) prob should use repo-commit|If refs in job, get SHA for ref else use downward api to get main SHA|
|job-version (dep)|Never set|
|pull|Pr number primary is testing, first pull in Spec Pull list|
|repo-commit|*unset* (but shouldn't be)|
|repos|For Ref, ExtraRef add Org/Repo: [Ref](https://github.com/kubernetes/test-infra/blob/4b5c7c99a851eb427f5c77bd0c8d11526f7b63c4/prow/apis/prowjobs/v1/types.go#L789) |
|node| *unset*|
|metadata| *misc*|
*Ex*
```
{
  "timestamp": 1595277241,
  "pull": "93264",
  "repos": {
    "kubernetes/kubernetes": "master:5feab0aa1e592ab413b461bc3ad08a6b74a427b4,93264:5dd9241d43f256984358354d1fec468f274f9ac4"
  },
  "metadata": {
    "links": {
      "resultstore": {
        "url": "https://source.cloud.google.com/results/invocations/20688dbb-eb32-47e6-8a49-34734e714f81/targets/test"
      }
    },
    "resultstore": "https://source.cloud.google.com/results/invocations/20688dbb-eb32-47e6-8a49-34734e714f81/targets/test"
  },
  "repo-version": "30f64c5b1fc57a3beb1476f9beb29280166954d1",
  "Pending": false
}
```

[`Finished.json` *PodUtils*](https://github.com/kubernetes/test-infra/blob/016edc15b8271c7528993cea0615cb11ecff201c/prow/sidecar/run.go#L209)
|Fields|Content|
|---|---|
|timestamp|epoch|
|passed|bool|
|result|SUCCESS, ABORTED, FAILURE|
|repo-version (dep)|*unset*|
|job-version (dep)|*unset*|
|revision (dep)|[SHA from Refs](https://github.com/kubernetes/test-infra/blob/4b5c7c99a851eb427f5c77bd0c8d11526f7b63c4/prow/pod-utils/downwardapi/jobspec.go#L163)|
|metadata| *unset*|
*Ex*
```
{
  "timestamp": 1595279434,
  "passed": true,
  "result": "SUCCESS",
  "revision": "5dd9241d43f256984358354d1fec468f274f9ac4"
}
```