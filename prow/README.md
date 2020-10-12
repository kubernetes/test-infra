# ![Prow](logo_horizontal_solid.png)

Prow is a Kubernetes based CI/CD system. Jobs can be triggered by various types of events and report their status to many different services. In addition to job execution, Prow provides GitHub automation in the form of policy enforcement, chat-ops via `/foo` style commands, and automatic PR merging.

See the [GoDoc](https://godoc.org/k8s.io/test-infra/prow) for library docs.
Please note that these libraries are intended for use by prow only, and we do
not make any attempt to preserve backwards compatibility.

For a brief overview of how Prow runs jobs take a look at ["Life of a Prow Job"](/prow/life_of_a_prow_job.md).

To see common Prow usage and interactions flow, see the pull request interactions [sequence diagram](https://raw.githubusercontent.com/kubernetes/test-infra/master/prow/docs/pr-interactions-sequence.svg?sanitize=true).

#### Functions and Features
* Job execution for testing, batch processing, artifact publishing.
    * GitHub events are used to trigger post-PR-merge (postsubmit) jobs and on-PR-update (presubmit) jobs.
    * Support for multiple execution platforms and source code review sites.
* Pluggable GitHub bot automation that implements `/foo` style commands and enforces configured policies/processes.
* GitHub merge automation with batch testing logic.
* Front end for viewing jobs, merge queue status, dynamically generated help information, and more.
* Automatic deployment of source control based config.
* Automatic GitHub org/repo administration configured in source control.
* Designed for multi-org scale with dozens of repositories. (The Kubernetes Prow instance uses only 1 GitHub bot token!)
* High availability as benefit of running on Kubernetes. (replication, load balancing, rolling updates...)
* JSON structured logs.
* Prometheus metrics.


## Documentation

### Getting started

* With your own Prow deployment: [`getting_started_deploy.md`](/prow/getting_started_deploy.md)
* With developing for Prow: [`getting_started_develop.md`](/prow/getting_started_develop.md)
* As a job author: [`jobs.md`](/prow/jobs.md)

### More details
- [Components](/prow/cmd/README.md)
- [Plugins](/prow/plugins/README.md)
- [ProwJobs](/prow/jobs.md)
- [Building, Testing, and Updating](/prow/build_test_update.md)
- [General Configuration](/prow/config/README.md)
- [Pod Utilities](/prow/pod-utilities.md)
- [Scaling Prow](/prow/scaling.md)
- [Tide](/prow/cmd/tide/README.md)
- [Metrics](/prow/metrics/README.md)
- ["Life of a Prow Job"](/prow/life_of_a_prow_job.md)
- [Getting more out of Prow](/prow/more_prow.md)

## Useful Talks

### Kuecon 2020 EU virtual

 - [Going Beyond CI/CD with Prow](https://youtu.be/qQvoImxHydk)

### KubeCon 2018 EU

- [Automation and the Kubernetes Contributor Experience](https://www.youtube.com/watch?v=BsIC7gPkH5M)
- [SIG Testing Deep Dive](https://www.youtube.com/watch?v=M32NIHRKaOI)

### KubeCon 2018 China

- [SIG Testing Intro](https://youtu.be/WFvC_VdkDFk)

### KubeCon 2018 Seattle

- [Behind you PR: K8s with K8s on K8s](https://www.youtube.com/watch?v=pz0lpl6h-Gc)
- [Using Prow for Testing Outside of K8s](https://www.youtube.com/watch?v=DBrkSC6nS8A)
- [Jenkins X (featuring Tide)](https://www.youtube.com/watch?v=IDEa8seAzVc)
- [SIG Testing Intro](https://www.youtube.com/watch?v=7-_O41W3FRU)
- [SIG Testing Deep Dive](https://www.youtube.com/watch?v=1rwiKDTJILY)

### Misc

- [Deploy Your Own Kubernetes Prow](https://www.youtube.com/watch?v=eMNwB96A1Qc)


## Prow in the wild

Prow is used by the following organizations and projects:
- [Kubernetes](https://prow.k8s.io)
  - This includes [kubernetes](https://github.com/kubernetes), [kubernetes-client](https://github.com/kubernetes-client), [kubernetes-csi](https://github.com/kubernetes-csi), and [kubernetes-sigs](https://github.com/kubernetes-sigs).
- [OpenShift](https://prow.svc.ci.openshift.org/)
  - This includes [openshift](https://github.com/openshift), [openshift-s2i](https://github.com/openshift-s2i), [operator-framework](https://github.com/operator-framework), and some repos in [containers](https://github.com/containers) and [heketi](https://github.com/heketi).
- [Istio](https://prow.istio.io/)
- [Knative](https://prow.knative.dev/)
- [Jetstack](https://prow.build-infra.jetstack.net/)
- [Kyma](https://status.build.kyma-project.io/)
- [Metal³](https://prow.apps.ci.metal3.io/)
- [Prometheus](http://prombench.prometheus.io/)
- [Caicloud](https://github.com/caicloud)
- [Kubeflow](https://github.com/kubeflow)
- [Azure AKS Engine](https://github.com/Azure/aks-engine/tree/master/.prowci)
- [tensorflow/minigo](https://github.com/tensorflow/minigo#automated-tests)
- [helm/charts](https://github.com/helm/charts)
- [Daisy(google compute image tools)](https://github.com/GoogleCloudPlatform/compute-image-tools/tree/master/test-infra#prow-and-gubenator)
- [KubeEdge (Kubernetes Native Edge Computing Framework)](https://github.com/kubeedge/kubeedge)
- [Volcano (Kubernetes Native Batch System)](https://github.com/volcano-sh/volcano)
- [Loodse](https://public-prow.loodse.com/)
- [Feast](https://github.com/gojek/feast)

[Jenkins X](https://jenkins-x.io/) uses [Prow as part of Serverless Jenkins](https://medium.com/@jdrawlings/serverless-jenkins-with-jenkins-x-9134cbfe6870).

## Contact us

If you need to contact the maintainers of Prow you have a few options:
1. Open an issue in the [kubernetes/test-infra](https://github.com/kubernetes/test-infra) repo.
1. Reach out to the `#prow` channel of the [Kubernetes Slack](https://github.com/kubernetes/community/tree/master/communication#social-media).
1. Contact one of the code owners in [prow/OWNERS](/prow/OWNERS) or in a more specifically scoped OWNERS file.

### Bots home
[@k8s-ci-robot](https://github.com/k8s-ci-robot) lives here and is the face of the Kubernetes Prow instance. Here is a [command list](https://go.k8s.io/bot-commands) for interacting with @k8s-ci-robot and other Prow bots.
