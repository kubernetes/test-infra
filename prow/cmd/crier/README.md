# Crier

Crier reports your prowjobs on their status changes.

## Usage / How to enable existing available reporters

For any reporter you want to use, you need to mount your prow configs and specify `--config-path` and `job-config-path`
flag as most of other prow controllers do.

### [Gerrit reporter](/prow/gerrit/reporter)

You can enable gerrit reporter in crier by specifying `--gerrit-workers=n` flag.

Similar to the [gerrit adapter](/prow/cmd/gerrit), you'll need to specify `--gerrit-projects` for
your gerrit projects, and also `--cookiefile` for the gerrit auth token (leave it unset for anonymous).

Gerrit reporter will send a gerrit code review, when all [gerrit adapter](/prow/cmd/gerrit)
scheduled prowjob finishes on a revision, aka, on `SuccessState`, `FailureState`, `AbortedState` or `ErrorState`.
It will also attach a report url so people can find logs of the job.

### [Pubsub reporter](/prow/pubsub/reporter)

You can enable pubsub reporter in crier by specifying `--pubsub-workers=n` flag.

You need to specify following labels in order for pubsub reporter to report your prowjob:

| Label                          | Description                                                                                                                               |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------|
| `"prow.k8s.io/pubsub.project"` | Your gcp project where pubsub channel lives                                                                                               |
| `"prow.k8s.io/pubsub.topic"`   | The [topic](https://cloud.google.com/pubsub/docs/publisher) of your pubsub message                                                        |
| `"prow.k8s.io/pubsub.runID"`   | A user assigned job id. It's tied to the prowjob, serves as a name tag and help user to differentiate results in multiple pubsub messages |

Pubsub reporter will report whenever prowjob has a state transition.

You can check the reported result by [list the pubsub topic](https://cloud.google.com/sdk/gcloud/reference/pubsub/topics/list).

### [GitHub reporter](/prow/github/reporter)

You can enable github reporter in crier by specifying `--github-workers=1` flag. (We only support single worker for github, due to [#13306](https://github.com/kubernetes/test-infra/issues/13306))

You also need to mount a github oauth token by specifying `--github-token-path` flag, which defaults to `/etc/github/oauth`.

If you have a [ghproxy](/ghproxy) deployed, also remember to point `--github-endpoint` to your ghproxy to avoid token throttle.

The actual report logic is in the [github report library](/prow/github/report) for your reference.

### [Slack reporter](/prow/slack/reporter)

You can enable the Slack reporter in crier by specifying the `--slack-workers=n` and `--slack-token-file=path-to-tokenfile` flags.

The `--slack-token-file` flag takes a path to a file containing a Slack [*OAuth Access Token*](https://api.slack.com/docs/oauth). The access token can be obtained after installing 
a [Slack app](https://api.slack.com/apps) for the *workspace*. The app should have the [`chat:write:bot`](https://api.slack.com/scopes/chat:write:bot) scope granted in order to [post messages](https://api.slack.com/methods/chat.postMessage) to the configured Slack *channel*.

In order for it to work, you must add the following to your `config.yaml`:

```
slack_reporter:
  # Default: None
  job_types_to_report:
  - presubmit
  - postsubmit
  - periodic
  - batch
  # Default: None
  job_states_to_report:
  - triggered
  - pending
  - success
  - failure
  - aborted
  - error
  channel: my-slack-channel
  # The template shown below is the default
  report_template: 'Job {{.Spec.Job}} of type {{.Spec.Type}} ended with state {{.Status.State}}. <{{.Status.URL}}|View logs>'
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

Both plank and crier will call into the [github report lib](prow/github/report) when a prowjob needs to be reported,
so as a user you only want to make one of them to report :-)

Before migrating, be sure plank is setting the [PrevReportStatus field](https://github.com/kubernetes/test-infra/blob/master/prow/apis/prowjobs/v1/types.go#L403)
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

You want to add a crier deployment, similar to ours [prow/cluster/crier_deployment.yaml](prow/cluster/crier_deployment.yaml),
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
