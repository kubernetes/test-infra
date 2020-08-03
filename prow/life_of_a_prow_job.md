# Life of a Prow Job

I comment `/test all` on a Pull Request (PR). In response, GitHub posts my comment to Prow, via [a webhook](https://developer.github.com/webhooks/). See examples for webhook payloads [here](https://github.com/kubernetes/test-infra/tree/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/cmd/phony/examples).

Prow's Kubernetes cluster uses an [ingress resource](https://kubernetes.io/docs/concepts/services-networking/ingress/) for terminating TLS, and routing traffic to the hook [service resource](https://kubernetes.io/docs/concepts/services-networking/service/). [This document](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/cluster/ingress.yaml) describes the configuration for the ingress resource. The ingress resource sends traffic to the "hook" service. [This document](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/cluster/hook_service.yaml) describes the configuration for the "hook" service.

The "hook" service routes traffic to the "hook" application. [This deployment resource](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/cluster/hook_deployment.yaml) defines the pods that "hook" is comprised of.

The pods for "hook" run [the "Hook" executable](https://github.com/kubernetes/test-infra/blob/42d4af367a2312d8facbb92f9669f7356d8b13f4/prow/cmd/hook/main.go#L95). "hook" listens for incoming http requests and translates them to "GitHub event objects". Afterwards, "hook" broadcasts these events to Prow Plugins. In the case of my `/test all` comment, "hook" builds an ["Issue Comment Event"](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/github/types.go#L116-L121).

Prow plugins receive 2 objects: 1) a GitHub event object, and 2) a ["client agent object"](https://github.com/kubernetes/test-infra/blob/42d4af367a2312d8facbb92f9669f7356d8b13f4/prow/plugins/plugins.go#L199). The Client agent object contains 7 clients: GitHub, Prow jobs, Kubernetes, Git, Slack, Owners, and Bugzilla. These 7 clients are initialized by "hook", during start-up. This is what "client agent objects" look like:

```go
type ClientAgent struct {
	GitHubClient     github.Client
	ProwJobClient    prowv1.ProwJobInterface
	KubernetesClient kubernetes.Interface
	GitClient        *git.Client
	SlackClient      *slack.Client
	OwnersClient     *repoowners.Client
	BugzillaClient   bugzilla.Client
}
```

"hook" [multiplexes events](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/cmd/hook/server.go#L40) by looking at "X-GitHub-Event", a custom http header. Afterwards, a [PluginAgent object](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/plugins/plugins.go#L86), initialized [during Hook's startup](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/cmd/hook/main.go#L128), selects plugins to handle events. See [events.go](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/cmd/hook/events.go#L17) for more details, and check [plugins.yaml](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/plugins.yaml) for a list of plugins per repo.

"hook" delivers an event that represents my `/test all` comment to the [Trigger plugin](https://github.com/kubernetes/test-infra/tree/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/plugins/trigger). The Trigger plugin validates the PR before running tests. One such validation is, for instance, that the author is a member of the organization or that the PR is labeled `ok-to-test`. The function called [`handleGenericCommentEvent` (implemented by `handleGenericComment`)](https://github.com/kubernetes/test-infra/blob/99b91b56b097e39d70cb1ae82c0b1cb57d98ac48/prow/plugins/trigger/generic-comment.go#L32) describes Trigger's logic.

Finally, `handleGenericComment` determines *presubmit* jobs to run. The list of jobs supplied by the `Config` object, in the `PluginClient` object, will be used to find suitable jobs.

Next, the trigger plugin talks to the Kubernetes API server and creates a [ProwJob Custom Resource](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/kube/prowjob.go#L50-L83) with the information from the issue comment.

Pod details aside, the resulting Prow job looks like this:

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
    - image: gcr.io/k8s-testimages/bazelbuild:0.11
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

Every thirty seconds, `cmd/plank` runs [`Sync`](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/plank/controller.go#L71). `Sync` runs [`syncKubernetesJob`](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/plank/controller.go#L233-L296) for each Prow job and pod. Because the above Prow job lacks a corresponding Kubernetes pod, `Sync` creates one in the [`test-pods`](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/cluster/test_pod_namespace.yaml) namespace. Finally, `Sync` updates the status line on the PR with a link to Gubernator. Gubernator handles real-time logs and results.

When the Prow job ends, the `syncKubernetesJob` method updates the ProwJob status to success and sets the status line on GitHub to success. The status update makes a green check-mark show up on the PR.

A day later, [`cmd/sinker`](https://github.com/kubernetes/test-infra/blob/c8829eef589a044126289cb5b4dc8e85db3ea22f/prow/cmd/sinker/main.go#L58-L92) notices that the job and pod are a day old and deletes them from the Kubernetes API server.
