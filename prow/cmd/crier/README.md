# Crier

Crier reports your prowjobs on their status changes.

## Usage / How to enable existing available reporters

For any reporter you want to use, you need to mount your prow configs and specify `--config-path` and `job-config-path`
flag as most of other prow controllers do.

### [Gerrit reporter](/prow/crier/reporters/gerrit)

You can enable gerrit reporter in crier by specifying `--gerrit-workers=n` flag.

Similar to the [gerrit adapter](/prow/cmd/gerrit), you'll need to specify `--gerrit-projects` for
your gerrit projects, and also `--cookiefile` for the gerrit auth token (leave it unset for anonymous).

Gerrit reporter will send an aggregated summary message, when all [gerrit adapter](/prow/cmd/gerrit)
scheduled prowjobs with the same report label finish on a revision.
It will also attach a report url so people can find logs of the job.

The reporter will also cast a +1/-1 vote on the `prow.k8s.io/gerrit-report-label` label of your prowjob,
or by default it will vote on `CodeReview` label. Where `+1` means all jobs on the patshset pass and `-1`
means one or more jobs failed on the patchset.

### [Pubsub reporter](/prow/crier/reporters/pubsub)

You can enable pubsub reporter in crier by specifying `--pubsub-workers=n` flag.

You need to specify following labels in order for pubsub reporter to report your prowjob:

| Label                          | Description                                                                                                                               |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------|
| `"prow.k8s.io/pubsub.project"` | Your gcp project where pubsub channel lives                                                                                               |
| `"prow.k8s.io/pubsub.topic"`   | The [topic](https://cloud.google.com/pubsub/docs/publisher) of your pubsub message                                                        |
| `"prow.k8s.io/pubsub.runID"`   | A user assigned job id. It's tied to the prowjob, serves as a name tag and help user to differentiate results in multiple pubsub messages |

Pubsub reporter will report whenever prowjob has a state transition.

You can check the reported result by [list the pubsub topic](https://cloud.google.com/sdk/gcloud/reference/pubsub/topics/list).

### [GitHub reporter](/prow/crier/reporters/github)

You can enable github reporter in crier by specifying `--github-workers=N` flag (N>0).

You also need to mount a github oauth token by specifying `--github-token-path` flag, which defaults to `/etc/github/oauth`.

If you have a [ghproxy](/ghproxy) deployed, also remember to point `--github-endpoint` to your ghproxy to avoid token throttle.

The actual report logic is in the [github report library](/prow/github/report) for your reference.

### [Slack reporter](/prow/crier/reporters/slack)

> **NOTE:** if enabling the slack reporter for the *first* time, Crier will message to the Slack channel for **all** ProwJobs matching the configured filtering criteria.

You can enable the Slack reporter in crier by specifying the `--slack-workers=n` and `--slack-token-file=path-to-tokenfile` flags.

The `--slack-token-file` flag takes a path to a file containing a Slack [**OAuth Access Token**](https://api.slack.com/docs/oauth).

The **OAuth Access Token** can be obtained as follows:
1. Navigate to: https://api.slack.com/apps.
1. Click **Create New App**.
1. Provide an **App Name** (e.g. Prow Slack Reporter) and **Development Slack Workspace** (e.g. Kubernetes).
1. Click **Permissions**.
1. Add the `chat:write.public` scope using the **Scopes / Bot Token Scopes** dropdown and **Save Changes**.
1. Click **Install App to Workspace**
1. Click **Allow** to authorize the Oauth scopes.
1. Copy the **OAuth Access Token**.

Once the *access token* is obtained, you can create a `secret` in the cluster using that value:

```shell
kubectl create secret generic slack-token --from-literal=token=< access token >
```

Furthermore, to make this token available to **Crier**, mount the *slack-token* `secret` using a `volume` and set the `--slack-token-file` flag in the deployment spec.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: crier
  labels:
    app: crier
spec:
  selector:
    matchLabels:
      app: crier
  template:
    metadata:
      labels:
        app: crier
    spec:
      containers:
      - name: crier
        image: gcr.io/k8s-prow/crier:v20200205-656133e91
        args:
        - --slack-workers=1
        - --slack-token-file=/etc/slack/token
        - --config-path=/etc/config/config.yaml
        - --dry-run=false
        volumeMounts:
        - mountPath: /etc/config
          name: config
          readOnly: true
        - name: slack
          mountPath: /etc/slack
          readOnly: true
      volumes:
      - name: slack
        secret:
          secretName: slack-token
      - name: config
        configMap:
          name: config
```

Additionally, in order for it to work with Prow you must add the following to your `config.yaml`:

> **NOTE:** `slack_reporter_configs` is a map of `org`, `org/repo`, or `*` (i.e. catch-all wildcard) to a set of slack reporter configs.

```yaml
slack_reporter_configs:

  # Wildcard (i.e. catch-all) slack config
  "*":
    # default: None
    job_types_to_report:
      - presubmit
      - postsubmit
    # default: None
    job_states_to_report:
      - failure
      - error
    # required
    channel: my-slack-channel
    # The template shown below is the default
    report_template: "Job {{.Spec.Job}} of type {{.Spec.Type}} ended with state {{.Status.State}}. <{{.Status.URL}}|View logs>"

  # "org/repo" slack config
  istio/proxy:
    job_types_to_report:
      - presubmit
    job_states_to_report:
      - error
    channel: istio-proxy-channel

  # "org" slack config
  istio:
    job_types_to_report:
      - periodic
    job_states_to_report:
      - failure
    channel: istio-channel
```

The Slack `channel` can be overridden at the ProwJob level via the `reporter_config.slack.channel` field:
```yaml
postsubmits:
  some-org/some-repo:
    - name: example-job
      decorate: true
      reporter_config:
        slack:
          channel: 'override-channel-name'
      spec:
        containers:
          - image: alpine
            command:
              - echo
```

## Implementation details

Crier supports multiple reporters, each reporter will become a crier controller. Controllers
will get prowjob change notifications from a [shared informer](https://github.com/kubernetes/client-go/blob/master/tools/cache/shared_informer.go), and you can specify `--num-workers` to change parallelism.

If you are interested in how client-go works under the hood, the details are explained
[in this doc](https://github.com/kubernetes/sample-controller/blob/master/docs/controller-client-go.md)


## Adding a new reporter

Each crier controller takes in a reporter.

Each reporter will implement the following interface:
```go
type reportClient interface {
	Report(pj *v1.ProwJob) error
	GetName() string
	ShouldReport(pj *v1.ProwJob) bool
}
```

`GetName` will return the name of your reporter, the name will be used as a key when we store previous
reported state for each prowjob.

`ShouldReport` will return if a prowjob should be handled by current reporter.

`Report` is the actual report logic happens. Return `nil` means report is successful, and the reported
state will be saved in the prowjob. Return an actual error if report fails, crier will re-add the prowjob
key to the shared cache and retry up to 5 times.

You can add a reporter that implements the above interface, and add a flag to turn it on/off in crier.

## Migration from plank for github report

Both plank and crier will call into the [github report lib](https://github.com/kubernetes/test-infra/tree/de3775a7480fe0a724baacf24a87cbf058cd9fd5/prow/github/report) when a prowjob needs to be reported,
so as a user you only want to make one of them to report :-)

To disable GitHub reporting in Plank, add the `--skip-report=true` flag to the Plank [deployment](https://github.com/kubernetes/test-infra/blob/de3775a7480fe0a724baacf24a87cbf058cd9fd5/prow/cluster/plank_deployment.yaml#L45).

Before migrating, be sure plank is setting the [PrevReportStates field](https://github.com/kubernetes/test-infra/blob/de3775a7480fe0a724baacf24a87cbf058cd9fd5/prow/apis/prowjobs/v1/types.go#L566)
by describing a finished presubmit prowjob. Plank started to set this field after commit [2118178](https://github.com/kubernetes/test-infra/pull/10975/commits/211817826fc3c4f3315a02e46f3d6aa35573d22f), if not, you want to upgrade your plank to a version includes this commit before moving forward.

you can check this entry by:
```sh
$ kubectl get prowjobs -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.prev_report_states.github-reporter}{"\n"}'
...
fafec9e1-3af2-11e9-ad1a-0a580a6c0d12	failure
fb027a97-3af2-11e9-ad1a-0a580a6c0d12	success
fb0499d3-3af2-11e9-ad1a-0a580a6c0d12	failure
fb05935f-3b2b-11e9-ad1a-0a580a6c0d12	success
fb05e1f1-3af2-11e9-ad1a-0a580a6c0d12	error
fb06c55c-3af2-11e9-ad1a-0a580a6c0d12	success
fb09e7d8-3abb-11e9-816a-0a580a6c0f7f	success


```

You want to add a crier deployment, similar to ours [config/prow/cluster/crier_deployment.yaml](https://github.com/kubernetes/test-infra/blob/de3775a7480fe0a724baacf24a87cbf058cd9fd5/prow/cluster/crier_deployment.yaml),
flags need to be specified:
- point `config-path` and `--job-config-path` to your prow config and job configs accordingly.
- Set `--github-worker` to be number of parallel github reporting threads you need
- Point `--github-endpoint` to ghproxy, if you have set that for plank
- Bind github oauth token as a secret and set `--github-token-path` if you've have that set for plank.

In your plank deployment, you can
- Remove the `--github-endpoint` flags
- Remove the github oauth secret, and `--github-token-path` flag if set
- Flip on `--skip-report`, so plank will skip the reporting logic

Both change should be deployed at the same time, if have an order preference, deploy crier first since report twice should just be a no-op.

We will send out an announcement when we cleaning up the report dependency from plank in later 2019.
