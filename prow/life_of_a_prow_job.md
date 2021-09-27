# Life of a Prow Job

NOTE: This document uses [5df7636b83cab54e248e550a31dbf1e4731197a6][prow-repo-sync-point] (July 21, 2021) as a reference point for all code links.

Let's pretend a user comments `/test all` on a Pull Request (PR).
In response, GitHub posts this comment to Prow via a [webhook][github-webhook].
See [examples for webhook payloads][sample-github-webhook-payloads].

Prow's Kubernetes cluster uses an [ingress resource][ingress-resource] for terminating TLS, and routes traffic to the **hook** [service resource][service-resource], finally sending the traffic to the **hook** application, which is defined as a [deployment][deployment-controller]:

* [This document][ingress-yaml] describes the configuration for the ingress resource.
* [This document][hook-service-yaml] describes the configuration for the **hook** service.
* [This document][hook-deployment-yaml] defines the pods for the  **hook** application.

The pods for **hook** run [the **hook** executable][hook-main].
**hook** listens for incoming HTTP requests and translates them to "GitHub event objects".
For example, in the case of the `/test all` comment from above, **hook** builds an [`GenericCommentEvent`][github-GenericCommentEvent].
Afterwards, **hook** broadcasts these events to Prow Plugins.

Prow Plugins receive 2 objects:

1) a GitHub event object, and
2) a [`ClientAgent`][plugins-ClientAgent] object.

The `ClientAgent` object contains the following clients:

- GitHub client
- Prow job client
- Kubernetes client
- BuildClusterCoreV1 clients
- Git client
- Slack client
- Owners client
- Bugzilla client
- Jira client

These clients are initialized by **hook**, during start-up.

**hook** [handles events][hook-ServeHTTP] by looking at [`X-GitHub-Event`][github-ValidateWebhook], a custom HTTP header.
Afterwards, a [`ConfigAgent` object][plugins-ConfigAgent], initialized [during **hook**'s startup][hook-initialize-configAgent], selects plugins to handle events.
See [githubeventserver.go][githubeventserver-handleEvent] for more details, and check [plugins.yaml][plugins-yaml] for a list of plugins per repo.

Going back to the example, **hook** delivers an event that represents the `/test all` comment to the [Trigger plugin][prow-plugins-trigger].
The Trigger plugin validates the PR before running tests.
One such validation is, for instance, that the author is a member of the organization or that the PR is labeled `ok-to-test`.
The function called [`handleGenericComment`][trigger-handleGenericComment] describes Trigger's logic.

If all conditions are met (`ok-to-test`, the comment is not a bot comment, etc.), `handleGenericComment` [determines][trigger-FilterPresubmits] which *presubmit* jobs to run.
The initial list of presubmit jobs to run (before being filtered down to those that qualify for this particular comment), is retrieved with [`getPresubmits`][trigger-handleGenericComment-getPresubmits].

Next, for each presubmit we want to run, the **trigger** plugin talks to the Kubernetes API server and creates a [`ProwJob`][api-ProwJob] with the information from the PR comment.
The `ProwJob` is primarily composed of the [`Spec`][api-ProwJobSpec] and [`Status`][api-ProwJobStatus] objects.

Pod details aside, a sample ProwJob might look like this:

```yaml
apiVersion: prow.k8s.io/v1
kind: ProwJob
metadata:
  name: 32456927-35d9-11e7-8d95-0a580a6c1504
spec:
  job: pull-test-infra-bazel
  decorate: true
  pod_spec:
    containers:
    - image: gcr.io/k8s-staging-test-infra/bazelbuild:latest-test-infra
  refs:
    base_ref: master
    base_sha: 064678510782db5b382df478bb374aaa32e577ea
    org: kubernetes
    pulls:
    - author: ixdy
      number: 2716
      sha: dc32ccc9ea3672ccc523b7cbaa8b00360b4183cd
    repo: test-infra
  type: presubmit
status:
  startTime: 2017-05-10T23:34:22.567457715Z
  state: triggered
```

[**prow-controller-manager**][prow-controller-manager] runs ProwJobs by launching them by creating a new Kubernetes pod.
It knows how to schedule new ProwJobs onto the cluster, responding to changes in the ProwJob or cluster health.

When the ProwJob finishes (the containers in the pod have finished running), **prow-controller-manager** updates the ProwJob.
[**crier**][crier] reports back the status of the ProwJob back to the various external services like GitHub (e.g., as a green check-mark on the PR where the original `/test all` comment was made).

A day later, [**sinker**][sinker] notices that the job and pod are a day old and [deletes them][sinker-clean] from the Kubernetes API server.

Here is a summary of the above:

1. User types in `/test all` as a comment into a GitHub PR.
1. GitHub sends a webhook (HTTP request) to Prow, to the `prow.k8s.io/hook` endpoint.
1. The request gets intercepted by the [ingress][ingress-yaml].
1. The ingress routes the request to the **hook** [service][hook-service-yaml].
1. The **hook** service in turn routes traffic to the **hook** *application*, defined as a [deployment][hook-deployment-yaml].
1. The container routes traffic to the **hook** [binary][hook-main] inside it.
1. **hook** binary [parses and validates the HTTP request][hook-ServeHTTP-ValidateWebhook] and [creates a GitHub event object][hook-ServeHTTP-demuxEvent].
1. **hook** binary sends the GitHub event object (in this case [`GenericCommentEvent`][github-GenericCommentEvent]) to [`handleGenericCommentEvent`][hook-handleGenericComment].
1. `handleGenericCommentEvent` sends the data to be handled by the [`handleEvent`][githubeventserver-handleEvent].
1. The data in the comment gets sent from **hook** to one of its many plugins, one of which is **trigger**. (The pattern is that **hook** constructs objects to be consumed by various plugins.)
1. **trigger** determines which presubmit jobs to run (because it sees the `/test` command in `/test all`).
1. **trigger** creates a ProwJob object!
1. **prow-controller-manager** creates a pod to start the ProwJob.
1. When the ProwJob's pod finishes, **prow-controller-manager** updates the ProwJob.
1. **crier** sees the updated ProwJob status and reports back to the GitHub PR (creating a new comment).
1. **sinker** cleans up the old pod from above and deletes it from the Kubernetes API server.

[github-webhook]: https://developer.github.com/webhooks/

[deployment-controller]: https://kubernetes.io/docs/concepts/workloads/controllers/deployment/
[ingress-resource]:      https://kubernetes.io/docs/concepts/services-networking/ingress/
[service-resource]:      https://kubernetes.io/docs/concepts/services-networking/service/

[hook-deployment-yaml]:                       https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/config/prow/cluster/hook_deployment.yaml
[hook-service-yaml]:                          https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/config/prow/cluster/hook_service.yaml
[ingress-yaml]:                               https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/config/prow/cluster/tls-ing_ingress.yaml
[plugins-yaml]:                               https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/config/prow/plugins.yaml
[api-ProwJob]:                                https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/apis/prowjobs/v1/types.go#L103
[api-ProwJobSpec]:                            https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/apis/prowjobs/v1/types.go#L115
[api-ProwJobStatus]:                          https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/apis/prowjobs/v1/types.go#L805
[hook-initialize-configAgent]:                https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/cmd/hook/main.go#L107
[hook-main]:                                  https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/cmd/hook/main.go#L99
[sinker-clean]:                               https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/cmd/sinker/main.go#L289
[sinker]:                                     https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/cmd/sinker/main.go#L94
[github-GenericCommentEvent]:                 https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/github/types.go#L1170
[github-ValidateWebhook]:                     https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/github/webhooks.go#L31
[githubeventserver-handleEvent]:              https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/githubeventserver/githubeventserver.go#L202
[hook-handleGenericComment]:                  https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/hook/events.go#L424
[hook-ServeHTTP]:                             https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/hook/server.go#L57
[hook-ServeHTTP-ValidateWebhook]:             https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/hook/server.go#L58
[hook-ServeHTTP-demuxEvent]:                  https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/hook/server.go#L72
[plugins-ClientAgent]:                        https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/plugins/plugins.go#L233
[plugins-ConfigAgent]:                        https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/plugins/plugins.go#L246
[trigger-FilterPresubmits]:                   https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/plugins/trigger/generic-comment.go#L147-L162
[trigger-handleGenericComment]:               https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/plugins/trigger/generic-comment.go#L32
[trigger-handleGenericComment-getPresubmits]: https://github.com/kubernetes/test-infra/blob/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/plugins/trigger/generic-comment.go#L56

[prow-repo-sync-point]:                       https://github.com/kubernetes/test-infra/tree/5df7636b83cab54e248e550a31dbf1e4731197a6
[crier]:                                      https://github.com/kubernetes/test-infra/tree/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/cmd/crier
[sample-github-webhook-payloads]:             https://github.com/kubernetes/test-infra/tree/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/cmd/phony/examples
[prow-controller-manager]:                    https://github.com/kubernetes/test-infra/tree/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/cmd/prow-controller-manager
[prow-plugins-trigger]:                       https://github.com/kubernetes/test-infra/tree/5df7636b83cab54e248e550a31dbf1e4731197a6/prow/plugins/trigger
