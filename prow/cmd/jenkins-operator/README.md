# Jenkins operator

`jenkins-operator` is a controller that enables Prow to use Jenkins
as a backend for running jobs.

## Jenkins configuration

A Jenkins master needs to be provided via `--jenkins-url` in order for
the operator to make requests to. By default, `--dry-run` is set to `true`
so the operator will not make any mutating requests to Jenkins, GitHub,
and Kubernetes, but you most probably want to set it to `false`.
The Jenkins operator expects to read the Prow configuration by default
in `/etc/config/config.yaml` which can be configured with `--config-path`.

The following stanza is config that can be optionally set in the Prow config file:

```yaml
jenkins_operators:
- max_concurrency: 150
  max_goroutines: 20
  job_url_template: 'https://storage-for-logs/{{if eq .Spec.Type "presubmit"}}pr-logs/pull{{else if eq .Spec.Type "batch"}}pr-logs/pull{{else}}logs{{end}}{{if ne .Spec.Refs.Repo "origin"}}/{{.Spec.Refs.Org}}_{{.Spec.Refs.Repo}}{{end}}{{if eq .Spec.Type "presubmit"}}/{{with index .Spec.Refs.Pulls 0}}{{.Number}}{{end}}{{else if eq .Spec.Type "batch"}}/batch{{end}}/{{.Spec.Job}}/{{.Status.BuildID}}/'
  report_template: '[Full PR test history](https://pr-history/{{if ne .Spec.Refs.Repo "origin"}}{{.Spec.Refs.Org}}_{{.Spec.Refs.Repo}}/{{end}}{{with index .Spec.Refs.Pulls 0}}{{.Number}}{{end}}).'
```

* `max_concurrency` is the maximum number of Jenkins builds that can
run in parallel, otherwise the operator is not going to start new builds.
Defaults to 0, which means no limit.
* `max_goroutines` is the maximum number of goroutines that the operator
will spin up to handle all Jenkins builds. Defaulted to 20.
* `job_url_template` is a Golang-templated URL that shows up in the Details
button next to the GitHub job status context. A ProwJob is provided as input
to the template.
* `report_template` is a Golang-templated message that shows up in GitHub in
case of a job failure. A ProwJob is provided as input to the template.

### Security

Various flavors of authentication are supported:
* basic auth, using `--jenkins-user` and `--jenkins-token-file`.
* OpenShift bearer token auth, using `--jenkins-bearer-token-file`.
* certificate-based auth, using `--cert-file`, `--key-file`, and
optionally `--ca-cert-file`.

Basic auth and bearer token are mutually exclusive options whereas
cert-based auth is complementary to both of them.

If [CSRF protection](https://wiki.jenkins.io/display/JENKINS/CSRF+Protection) is enabled in Jenkins, `--csrf-protect=true`
needs to be used on the operator's side to allow Prow to work correctly.

### Logs

Apart from a controller, the Jenkins operator also runs a http server
to serve Jenkins logs. You can configure the Prow frontend to show
Jenkins logs with the following Prow config:
```yaml
deck:
  external_agent_logs:
  - agent: jenkins
    url_template: 'http://jenkins-operator/job/{{.Spec.Job}}/{{.Status.BuildID}}/consoleText'
```

Deck uses `url_template` to contact jenkins-operator when a user
clicks the `Build log` button of a Jenkins job (`agent: jenkins`).
`jenkins-operator` forwards the request to Jenkins and serves back
the response.

**NOTE:** Deck will display the `Build log` button on the main page when the agent is not `kubernetes`
regardless the external agent log was configured on the server side. Deck has no way to know if the server
side configuration is consistent when rendering jobs on the main page.

## Job configuration

Below follows the Prow configuration for a Jenkins job:
```yaml
presubmits:
  org/repo:
  - name: pull-request-unit
    agent: jenkins
    always_run: true
    context: ci/prow/unit
    rerun_command: "/test unit"
    trigger: "((?m)^/test( all| unit),?(\\s+|$))"
```

You can read more about the different types of Prow jobs [elsewhere](https://github.com/kubernetes/test-infra/tree/master/prow#how-to-add-new-jobs).
What is interesting for us here is the `agent` field which needs to
be set to `jenkins` in order for jobs to be dispatched to Jenkins and
`name` which is the name of the job inside Jenkins.

## Sharding

Sharding of Jenkins jobs is supported via Kubernetes labels and label
selectors. This enables Prow to work with multiple Jenkins masters.
Three places need to be configured in order to use sharding:
* `--label-selector` in the Jenkins operator.
* `label_selector` in `jenkins_operators` in the Prow config.
* `labels` in the job config.

For example, one would set the following options:
* `--label-selector=master=jenkins-master` in a Jenkins operator.

This option forces the operator to list all ProwJobs with `master=jenkins-master`.

* `label_selector: master=jenkins-master` in the Prow config.
```yaml
jenkins_operators:
- label_selector: master=jenkins-master
  max_concurrency: 150
  max_goroutines: 20
```

`jenkins_operators` in the Prow config can be read by multiple running operators
and based on `label_selector`, each operator knows which config stanza does it
need to use. Thus, `--label-selector` and `label_selector` need to match exactly.

* `labels: jenkins-master` in the job config.

```yaml
presubmits:
  org/repo:
  - name: pull-request-unit
    agent: jenkins
    labels:
      master: jenkins-master
    always_run: true
    context: ci/prow/unit
    rerun_command: "/test unit"
    trigger: "((?m)^/test( all| unit),?(\\s+|$))"
```

Labels in the job config are set in ProwJobs during their creation.

## Kubernetes client

The Jenkins operator acts as a Kubernetes client since it manages ProwJobs
backed by Jenkins builds. It is expected to run as a pod inside a Kubernetes
cluster and so it uses the in-cluster client config.

## GitHub integration

The operator needs to talk to GitHub for updating commit statuses and
adding comments about failed tests. Note that this functionality may
potentially move into its own service, then the Jenkins operator will
not need to contact the GitHub API. The required options are already
defaulted:
* `github-token-path` set to `/etc/github/oauth`. This is the GitHub bot
oauth token that is used for updating job statuses and adding comments
in GitHub.
* `github-endpoint` set to `https://api.github.com`.


## Prometheus support

The following Prometheus metrics are exposed by the operator:

* `jenkins_requests` is the number of Jenkins requests made.
  - `verb` is the type of request (`GET`, `POST`)
  - `handler` is the path of the request, usually containing a
   job name (eg. `job/test-pull-request-unit`).
  - `code` is the status code of the request (`200`, `404`, etc.).
* `jenkins_request_retries` is the number of Jenkins request
retries made.
* `jenkins_request_latency` is the time for a request to roundtrip
between the operator and Jenkins.
* `resync_period_seconds` is the time the operator takes to complete
one reconciliation loop.
* `prowjobs` is the number of Jenkins prowjobs in the system.
  - `job_name` is the name of the job.
  - `type` is the type of the prowjob: presubmit, postsubmit, periodic, batch
  - `state` is the state of the prowjob: triggered, pending, success, failure, aborted, error

If a push gateway needs to be used it can be configured in the Prow config:
```yaml
push_gateway:
  endpoint: http://prometheus-push-gateway
  interval: 1m
```
